import functools
import hashlib
import http.server
import os
from pathlib import Path
import subprocess
import tempfile
import threading
import unittest


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_args):
        pass


class InstallLearnerFrontendTest(unittest.TestCase):
    def test_frontend_install_and_rollback(self):
        installer = Path(__file__).with_name('install-learner-frontend.sh')
        for scenario in ('success', 'bad_checksum', 'health_failure'):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                app = root / 'app'
                dist = app / 'releases' / 'old' / 'front' / 'dist'
                (dist / 'assets').mkdir(parents=True)
                (dist / 'kuromoji-dict').mkdir()
                (dist / 'index.html').write_text('old index')
                (dist / 'assets' / 'old.js').write_text('old bundle')
                (dist / 'kuromoji-dict' / 'base.dat.gz').write_bytes(b'existing dictionary')
                (app / 'current').symlink_to(app / 'releases' / 'old')
                stage = root / 'stage'
                (stage / 'assets').mkdir(parents=True)
                (stage / 'index.html').write_text('<script src="/assets/new.js"></script>')
                (stage / 'assets' / 'new.js').write_text('new bundle')
                entries = []
                for name in ('index.html', 'assets/new.js'):
                    digest = hashlib.sha256((stage / name).read_bytes()).hexdigest()
                    entries.append(f'{digest}  {name}\n')
                (stage / 'manifest.sha256').write_text(''.join(entries))
                if scenario == 'bad_checksum':
                    (stage / 'assets' / 'new.js').write_text('corrupted bundle')
                served = root / 'unhealthy' if scenario == 'health_failure' else dist
                served.mkdir(exist_ok=True)
                handler = functools.partial(QuietHandler, directory=str(served))
                with http.server.ThreadingHTTPServer(('127.0.0.1', 0), handler) as server:
                    thread = threading.Thread(target=server.serve_forever, daemon=True)
                    thread.start()
                    try:
                        env = dict(os.environ, APP_DIR=str(app), RELEASE_ID='a' * 40,
                                   FRONTEND_CHECK_URL=f'http://127.0.0.1:{server.server_port}/')
                        result = subprocess.run(['bash', str(installer), str(stage)], env=env,
                                                capture_output=True, text=True, timeout=20)
                    finally:
                        server.shutdown()
                        thread.join()
                expected_index = (stage / 'index.html').read_text() if scenario == 'success' else 'old index'
                self.assertEqual(result.returncode == 0, scenario == 'success', result.stdout + result.stderr)
                self.assertEqual((dist / 'index.html').read_text(), expected_index)
                self.assertEqual((dist / 'assets' / 'old.js').read_text(), 'old bundle')
                self.assertEqual((dist / 'kuromoji-dict' / 'base.dat.gz').read_bytes(), b'existing dictionary')
                self.assertEqual((app / 'current').resolve(), (app / 'releases' / 'old').resolve())
                if scenario == 'success':
                    self.assertEqual((dist / 'assets' / 'new.js').read_text(), 'new bundle')


if __name__ == '__main__':
    unittest.main()
