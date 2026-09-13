#!/usr/bin/env python3
import argparse
import base64
import ctypes
import json
from pathlib import Path

PLUGIN_ID = "muse-tools-stripper"
REPOSITORY = "https://github.com/ahoo/cpa-plugin-muse-tools-stripper"
MAX_CGO_BYTES_LENGTH = (1 << 31) - 1


class Buffer(ctypes.Structure):
    _fields_ = [("ptr", ctypes.c_void_p), ("len", ctypes.c_size_t)]


def load_functions(path: Path):
    library = ctypes.CDLL(str(path.resolve()))
    call = library.cliproxyPluginCall
    call.argtypes = [
        ctypes.c_char_p,
        ctypes.POINTER(ctypes.c_uint8),
        ctypes.c_size_t,
        ctypes.POINTER(Buffer),
    ]
    call.restype = ctypes.c_int
    free = library.cliproxyPluginFree
    free.argtypes = [ctypes.c_void_p, ctypes.c_size_t]
    free.restype = None
    return library, call, free


def invoke(call, free, method: str | None, payload: bytes | None = None, declared_length: int | None = None):
    raw = payload or b""
    request = (ctypes.c_uint8 * len(raw)).from_buffer_copy(raw) if payload else None
    response = Buffer()
    length = len(raw) if declared_length is None else declared_length
    encoded_method = method.encode() if method is not None else None
    status = call(encoded_method, request, length, ctypes.byref(response))
    try:
        data = ctypes.string_at(response.ptr, response.len) if response.ptr else b""
    finally:
        if response.ptr:
            free(response.ptr, response.len)
    if not data:
        raise SystemExit(f"{method or 'nil method'} returned an empty response (status={status})")
    try:
        return status, json.loads(data)
    except json.JSONDecodeError as error:
        raise SystemExit(f"{method or 'nil method'} returned invalid JSON (status={status}): {error}") from error


def transform_payload(method: str, model: str, body: bytes) -> bytes:
    encoded_body = base64.b64encode(body).decode()
    if method == "request.normalize":
        request = {
            "FromFormat": "claude",
            "ToFormat": "codex",
            "Model": model,
            "Body": encoded_body,
        }
    else:
        request = {
            "SourceFormat": "codex",
            "ToFormat": "",
            "Model": model,
            "RequestedModel": model,
            "Body": encoded_body,
        }
    return json.dumps(request, separators=(",", ":")).encode()


def decoded_result_body(envelope: dict) -> bytes:
    encoded = envelope.get("result", {}).get("Body")
    if not encoded:
        return b""
    return base64.b64decode(encoded, validate=True)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--library", required=True, type=Path)
    parser.add_argument("--version", required=True)
    args = parser.parse_args()

    _library, call, free = load_functions(args.library)
    status, envelope = invoke(call, free, "plugin.register")
    if status != 0 or not envelope.get("ok"):
        raise SystemExit(f"registration failed: status={status} envelope={envelope}")

    registration = envelope["result"]
    metadata = registration["metadata"]
    expected = {
        "Name": PLUGIN_ID,
        "Version": args.version,
        "Author": "ahoo",
        "GitHubRepository": REPOSITORY,
        "Logo": "",
        "ConfigFields": [],
    }
    if metadata != expected:
        raise SystemExit(f"registration metadata mismatch: {metadata!r}")
    if registration.get("schema_version") != 1:
        raise SystemExit(f"unexpected schema version: {registration!r}")
    expected_capabilities = {"request_interceptor": True, "request_normalizer": True}
    if registration.get("capabilities") != expected_capabilities:
        raise SystemExit(f"unexpected capabilities: {registration!r}")

    guards = (
        ("nil-method", None, 0, "invalid_method"),
        ("nil-buffer", "request.normalize", 1, "invalid_request"),
        ("oversized", "request.normalize", MAX_CGO_BYTES_LENGTH + 1, "request_too_large"),
    )
    for label, method, length, error_code in guards:
        status, envelope = invoke(call, free, method, declared_length=length)
        if status != 1 or envelope.get("ok") or envelope.get("error", {}).get("code") != error_code:
            raise SystemExit(f"{label} guard failed: status={status} envelope={envelope}")

    body = b'{"model":"muse-free","input":[{"type":"additional_tools"},{"type":"message"}]}'
    for method in ("request.intercept_before", "request.intercept_after", "request.normalize"):
        status, envelope = invoke(call, free, method, transform_payload(method, "muse-free", body))
        transformed = decoded_result_body(envelope)
        if status != 0 or not envelope.get("ok") or b"additional_tools" in transformed or b'"message"' not in transformed:
            raise SystemExit(f"{method} free-tier behavior failed: status={status} envelope={envelope}")

    status, envelope = invoke(
        call,
        free,
        "request.normalize",
        transform_payload("request.normalize", "muse-spark-1.3-contributor", body),
    )
    if status != 0 or not envelope.get("ok") or decoded_result_body(envelope) != body:
        raise SystemExit(f"paid-model passthrough failed: status={status} envelope={envelope}")

    print(f"verified {PLUGIN_ID} registration {args.version}, ABI guards, and filtering behavior")


if __name__ == "__main__":
    main()
