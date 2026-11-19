#!/usr/bin/env python3
"""Patch the NAS whisper.cpp server to isolate changes in encoder context size.

Every anchor must match exactly once. A source mismatch or a second application
fails before writing, so an incompatible local base image cannot build silently.
"""

from pathlib import Path
import sys


def patch(source: str) -> str:
    marker = "japanese-learning-app: isolate changing audio context"
    if marker in source:
        raise ValueError("context isolation patch is already present")

    default = "    whisper_params default_params = params;\n"
    request = "        get_req_parameters(req, params);\n"
    load = "        ctx = whisper_init_from_file_with_params(model.c_str(), cparams);"
    ready = "        state.store(SERVER_STATE_READY);"
    route = "svr->Post(sparams.request_path + sparams.inference_path"
    load_route = 'svr->Post(sparams.request_path + "/load"'
    for name, anchor in (("defaults", default), ("request", request),
                         ("model load", load), ("model ready", ready),
                         ("inference route", route), ("load route", load_route)):
        if source.count(anchor) != 1:
            raise ValueError(f"{name} anchor must appear once; found {source.count(anchor)}")

    inference_section = source[source.index(route):source.index(request)]
    load_section = source[source.index(load_route):source.index(ready)]
    mutex = "std::lock_guard<std::mutex> lock(whisper_mutex);"
    if mutex not in inference_section or mutex not in load_section:
        raise ValueError("both inference and model loading must hold whisper_mutex")
    if not source.index(load_route) < source.index(load) < source.index(ready):
        raise ValueError("model ready anchor must follow successful model loading")

    declarations = r'''    // japanese-learning-app: isolate changing audio context
    int last_effective_ctx = -1;
    std::string active_model_path = default_params.model;
'''
    isolation = r'''        // Rebuild only when the effective encoder window changes. The
        // mutex above keeps model loading and inference on one valid context.
        const int effective_ctx = params.audio_ctx > 0 ? params.audio_ctx : whisper_n_audio_ctx(ctx);
        if (last_effective_ctx >= 0 && effective_ctx != last_effective_ctx) {
            fprintf(stderr, "audio_ctx changed %d -> %d; rebuilding model state\n",
                    last_effective_ctx, effective_ctx);
            auto * new_ctx = whisper_init_from_file_with_params(active_model_path.c_str(), cparams);
            if (new_ctx == nullptr) {
                fprintf(stderr, "error: failed to reinitialize model; keeping previous context\n");
                res.status = 500;
                res.set_content("{\"error\":\"failed to reinitialize speech context\"}", "application/json");
                return;
            }
            // Match normal startup. In CPU-only builds this helper is a stub.
            whisper_ctx_init_openvino_encoder(new_ctx, nullptr, params.openvino_encode_device.c_str(), nullptr);
            whisper_free(ctx);
            ctx = new_ctx;
        }
        last_effective_ctx = effective_ctx;
'''
    patched = source.replace(default, default + declarations, 1)
    patched = patched.replace(request, request + isolation, 1)
    patched = patched.replace(ready, "        active_model_path = model;\n"
                              "        last_effective_ctx = -1;\n" + ready, 1)
    return patched


def main() -> None:
    path = Path(sys.argv[1] if len(sys.argv) > 1 else "/app/examples/server/server.cpp")
    try:
        original = path.read_text(encoding="utf-8")
        patched = patch(original)
        path.write_text(patched, encoding="utf-8")
    except (OSError, ValueError) as error:
        raise SystemExit(f"whisper.cpp isolation patch failed: {error}") from error
    print(f"Applied encoder-context isolation to {path}")


if __name__ == "__main__":
    main()
