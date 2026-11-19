"""Exercise the exact private-config loader embedded in the release workflow."""

import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import textwrap
import unittest


ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = (ROOT / ".github/workflows/deploy.yml").read_text()
BLOCK = textwrap.dedent(WORKFLOW.split("# BEGIN FLOWSPACE DEPLOY CONFIG", 1)[1].split("# END FLOWSPACE DEPLOY CONFIG", 1)[0])
FUNCTIONS = BLOCK.split("\nload_flowspace_private_config /home/ubuntu/", 1)[0]
VALID = {
    "FLOWSPACE_BASE_URL": "https://flowspace.example.test/app",
    "FLOWSPACE_SOURCE_INSTANCE_ID": "japanese-production",
    "FLOWSPACE_INTEGRATION_MASTER_KEY": "1" * 64,
}


class FlowSpaceDeployConfigTest(unittest.TestCase):
    def execute(self, environment=None, file_values=None, extra_lines="", mode=0o600, assertion=""):
        with tempfile.TemporaryDirectory() as temporary:
            config = Path(temporary) / "flowspace.env"
            marker = Path(temporary) / "executed"
            if file_values is not None:
                config.write_text("".join(f"{name}={value}\n" for name, value in file_values.items()) + extra_lines.replace("MARKER", str(marker)))
                config.chmod(mode)
            command = FUNCTIONS + f"\nload_flowspace_private_config {shlex.quote(str(config))}\nvalidate_flowspace_deploy_config\n" + assertion
            result = subprocess.run(["bash", "-eu", "-c", command], env={"PATH": os.environ["PATH"], **(environment or {})}, capture_output=True, text=True)
            self.assertFalse(marker.exists(), "private config was executed as shell code")
            self.assertNotIn(VALID["FLOWSPACE_INTEGRATION_MASTER_KEY"], result.stdout + result.stderr)
            return result

    def test_actions_configuration_works_without_a_private_file(self):
        self.assertEqual(self.execute(environment=VALID).returncode, 0)

    def test_private_file_supplies_only_missing_whitelisted_values(self):
        result = self.execute(environment={"FLOWSPACE_BASE_URL": "https://override.example.test"}, file_values=VALID, extra_lines='UNRELATED=$(touch MARKER)\nexport UNRELATED_TOO=bad\n', assertion='[ "$FLOWSPACE_BASE_URL" = "https://override.example.test" ]\n[ -z "${UNRELATED:-}" ]\n')
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_missing_or_partial_configuration_fails(self):
        for values in [{}, {"FLOWSPACE_BASE_URL": VALID["FLOWSPACE_BASE_URL"]}]:
            with self.subTest(values=values):
                self.assertNotEqual(self.execute(environment=values).returncode, 0)

    def test_production_url_rejects_plain_http_credentials_and_bad_urls(self):
        for base in ["http://127.0.0.1:4200", "https://", "https://user:pass@example.test", "https://example.test?token=value", "https://example.test#fragment", "https://example.test:bad"]:
            with self.subTest(base=base):
                self.assertNotEqual(self.execute(environment={**VALID, "FLOWSPACE_BASE_URL": base}).returncode, 0)

    def test_instance_and_master_key_are_validated_without_printing_values(self):
        for values in [{"FLOWSPACE_SOURCE_INSTANCE_ID": "bad instance"}, {"FLOWSPACE_INTEGRATION_MASTER_KEY": "short"}, {"FLOWSPACE_INTEGRATION_MASTER_KEY": "x" * 64}]:
            with self.subTest(field=next(iter(values))):
                self.assertNotEqual(self.execute(environment={**VALID, **values}).returncode, 0)

    def test_private_file_permissions_and_duplicate_fields_fail_closed(self):
        self.assertNotEqual(self.execute(file_values=VALID, mode=0o644).returncode, 0)
        self.assertNotEqual(self.execute(file_values=VALID, extra_lines="FLOWSPACE_BASE_URL=https://duplicate.example.test\n").returncode, 0)

    def test_configuration_is_validated_before_existing_services_are_stopped(self):
        self.assertLess(WORKFLOW.index("# END FLOWSPACE DEPLOY CONFIG"), WORKFLOW.index('echo "Stop legacy host processes"'))
        for name in VALID:
            self.assertIn(f'{name}="${name}" \\', WORKFLOW)
        self.assertIn("/home/ubuntu/.config/japanese-learning-app/flowspace.env", WORKFLOW)

    def test_literal_shell_blocks_parse(self):
        lines = WORKFLOW.splitlines()
        for index, line in enumerate(lines):
            if line.strip() not in {"run: |", "script: |"}:
                continue
            indentation = len(line) - len(line.lstrip())
            block = []
            for content in lines[index + 1:]:
                if content.strip() and len(content) - len(content.lstrip()) <= indentation:
                    break
                block.append(content)
            result = subprocess.run(["bash", "-n"], input=textwrap.dedent("\n".join(block)), capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()
