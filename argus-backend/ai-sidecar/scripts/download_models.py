from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import sys
import urllib.request
import uuid
import zipfile
from pathlib import Path
from typing import Any


class ManifestError(RuntimeError):
    pass


def download_from_manifest(
    manifest_path: Path | str,
    models_path: Path | str | None = None,
    *,
    force: bool = False,
    check_only: bool = False,
    require_sha256: bool = False,
) -> list[Path]:
    manifest_path = Path(manifest_path)
    manifest = _read_manifest(manifest_path)
    output_dir = Path(models_path or manifest.get("models_path") or "models")
    output_dir.mkdir(parents=True, exist_ok=True)

    written: list[Path] = []
    for entry in manifest.get("models", []):
        target = output_dir / _required(entry, "filename")
        expected_sha256 = _entry_value(entry, "sha256", "sha256_env")
        min_bytes = int(entry.get("min_bytes") or 1)

        if check_only:
            _validate_existing(target, expected_sha256, min_bytes, require_sha256=require_sha256)
            continue

        if target.exists() and not force:
            _validate_existing(target, expected_sha256, min_bytes, require_sha256=require_sha256)
            continue

        url = _entry_value(entry, "download_url", "download_url_env")
        if not url:
            raise ManifestError(
                f"{entry.get('name', target.name)} has no download URL. "
                f"Set {entry.get('download_url_env', 'download_url')} or place {target} manually."
            )

        tmp = output_dir / f"argus-model-download-{uuid.uuid4().hex}"
        tmp.mkdir(parents=True, exist_ok=False)
        try:
            downloaded = tmp / "source"
            _download_url(url, downloaded)

            if _is_zip(downloaded, url):
                extracted = tmp / "extracted.onnx"
                _extract_archive_member(downloaded, entry, extracted)
                candidate = extracted
            else:
                candidate = downloaded

            _validate_candidate(candidate, expected_sha256, min_bytes, require_sha256=require_sha256)
            _atomic_replace(candidate, target)
            written.append(target)
        finally:
            shutil.rmtree(tmp, ignore_errors=True)

    return written


def _read_manifest(path: Path) -> dict[str, Any]:
    try:
        manifest = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise ManifestError(f"manifest file not found: {path}") from exc
    except json.JSONDecodeError as exc:
        raise ManifestError(f"invalid manifest JSON in {path}: {exc}") from exc

    if not isinstance(manifest.get("models"), list):
        raise ManifestError("manifest must contain a models array")
    return manifest


def _required(entry: dict[str, Any], key: str) -> str:
    value = entry.get(key)
    if not isinstance(value, str) or not value.strip():
        raise ManifestError(f"model entry {entry.get('name', '<unnamed>')} is missing {key}")
    return value.strip()


def _entry_value(entry: dict[str, Any], literal_key: str, env_key: str) -> str:
    env_name = entry.get(env_key)
    if isinstance(env_name, str) and env_name.strip():
        raw = os.getenv(env_name.strip())
        if raw:
            return raw.strip()

    raw = entry.get(literal_key)
    if raw is None:
        return ""
    return str(raw).strip()


def _download_url(url: str, destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    try:
        with urllib.request.urlopen(url, timeout=120) as response, destination.open("wb") as out:
            shutil.copyfileobj(response, out)
    except Exception as exc:
        raise ManifestError(f"failed to download {url}: {exc}") from exc


def _is_zip(path: Path, url: str) -> bool:
    return url.lower().split("?", 1)[0].endswith(".zip") or zipfile.is_zipfile(path)


def _extract_archive_member(archive: Path, entry: dict[str, Any], destination: Path) -> None:
    member = _entry_value(entry, "archive_member", "archive_member_env")
    suffix = str(entry.get("archive_member_suffix") or "").strip()

    with zipfile.ZipFile(archive) as zf:
        names = [name for name in zf.namelist() if not name.endswith("/")]
        if not member:
            matches = [name for name in names if suffix and name.endswith(suffix)]
            if len(matches) == 1:
                member = matches[0]
            elif len(matches) > 1:
                raise ManifestError(f"multiple archive members match suffix {suffix}: {matches}")
            else:
                raise ManifestError(f"archive_member is required for {entry.get('name', archive.name)}")

        if member not in names:
            raise ManifestError(f"archive member {member} not found in {archive}")

        with zf.open(member) as src, destination.open("wb") as out:
            shutil.copyfileobj(src, out)


def _validate_existing(
    path: Path,
    expected_sha256: str,
    min_bytes: int,
    *,
    require_sha256: bool,
) -> None:
    if not path.is_file():
        raise ManifestError(f"model file is missing: {path}")
    _validate_candidate(path, expected_sha256, min_bytes, require_sha256=require_sha256)


def _validate_candidate(
    path: Path,
    expected_sha256: str,
    min_bytes: int,
    *,
    require_sha256: bool,
) -> None:
    size = path.stat().st_size
    if size < min_bytes:
        raise ManifestError(f"{path} is too small: {size} bytes, expected at least {min_bytes}")

    if not expected_sha256:
        if require_sha256:
            raise ManifestError(f"sha256 is required but not configured for {path.name}")
        return

    actual = _sha256(path)
    if actual.lower() != expected_sha256.lower():
        raise ManifestError(f"{path} sha256 mismatch: expected {expected_sha256}, got {actual}")


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _atomic_replace(source: Path, target: Path) -> None:
    target.parent.mkdir(parents=True, exist_ok=True)
    tmp_target = target.with_name(f"{target.name}.tmp")
    shutil.copyfile(source, tmp_target)
    try:
        os.replace(tmp_target, target)
    except PermissionError:
        # Windows sandboxed filesystems can reject rename-to-ONNX operations even
        # when direct writes are allowed. Production Linux still uses os.replace.
        shutil.copyfile(tmp_target, target)
        try:
            tmp_target.unlink(missing_ok=True)
        except PermissionError:
            pass


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Download and validate Argus ONNX model files.")
    parser.add_argument("--manifest", type=Path, default=Path(__file__).resolve().parents[1] / "model_manifest.json")
    parser.add_argument("--models-path", type=Path, default=Path(os.getenv("ONNX_MODELS_PATH", "models")))
    parser.add_argument("--force", action="store_true", help="Replace existing model files.")
    parser.add_argument("--check-only", action="store_true", help="Only validate files already present in models-path.")
    parser.add_argument("--require-sha256", action="store_true", help="Fail if a manifest entry has no SHA256 value.")
    args = parser.parse_args(argv)

    try:
        written = download_from_manifest(
            args.manifest,
            args.models_path,
            force=args.force,
            check_only=args.check_only,
            require_sha256=args.require_sha256,
        )
    except ManifestError as exc:
        print(f"model manifest error: {exc}", file=sys.stderr)
        return 1

    if args.check_only:
        print(f"validated models in {args.models_path}")
    elif written:
        for path in written:
            print(f"wrote {path}")
    else:
        print(f"models already present in {args.models_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
