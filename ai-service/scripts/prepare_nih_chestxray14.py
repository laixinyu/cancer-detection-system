#!/usr/bin/env python3
"""
Prepare NIH ChestXray14 dataset:
1) download archives/metadata (resume supported)
2) optional extract
3) integrity verification against Data_Entry_2017*.csv
"""

import argparse
import csv
import json
import os
import tarfile
import time
import urllib.request
from urllib.error import HTTPError
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List, Optional, Tuple


IMAGE_URLS = [
    "https://nihcc.box.com/shared/static/vfk49d74nhbxq3nqjg0900w5nvkorp5c.gz",
    "https://nihcc.box.com/shared/static/i28rlmbvmfjbl8p2n3ril0pptcmcu9d1.gz",
    "https://nihcc.box.com/shared/static/f1t00wrtdk94satdfb9olcolqx20z2jp.gz",
    "https://nihcc.box.com/shared/static/0aowwzs5lhjrceb3qp67ahp0rd1l1etg.gz",
    "https://nihcc.box.com/shared/static/v5e3goj22zr6h8tzualxfsqlqaygfbsn.gz",
    "https://nihcc.box.com/shared/static/asi7ikud9jwnkrnkj99jnpfkjdes7l6l.gz",
    "https://nihcc.box.com/shared/static/jn1b4mw4n6lnh74ovmcjb8y48h8xj07n.gz",
    "https://nihcc.box.com/shared/static/tvpxmn7qyrgl0w8wfh9kqfjskv6nmm1j.gz",
    "https://nihcc.box.com/shared/static/upyy3ml7qdumlgk2rfcvlb9k6gvqq2pj.gz",
    "https://nihcc.box.com/shared/static/l6nilvfa9cg3s28tqv1qc1olm3gnz54p.gz",
    "https://nihcc.box.com/shared/static/hhq8fkdgvcari67vfhs7ppg2w6ni4jze.gz",
    "https://nihcc.box.com/shared/static/ioqwiy20ihqwyr8pf4c24eazhh281pbu.gz",
]

METADATA_URLS = {
    # Must use full metadata files (not dummy sample paths).
    "Data_Entry_2017_v2020.csv": "https://huggingface.co/datasets/alkzar90/NIH-Chest-X-ray-dataset/raw/main/data/Data_Entry_2017_v2020.csv",
    "train_val_list.txt": "https://huggingface.co/datasets/alkzar90/NIH-Chest-X-ray-dataset/raw/main/data/train_val_list.txt",
    "test_list.txt": "https://huggingface.co/datasets/alkzar90/NIH-Chest-X-ray-dataset/raw/main/data/test_list.txt",
}


@dataclass
class DownloadItem:
    url: str
    path: Path


def log(msg: str) -> None:
    print(msg, flush=True)


def infer_archive_name(index: int) -> str:
    return f"images_{index:03d}.tar.gz"


def ensure_dirs(root: Path) -> Tuple[Path, Path, Path]:
    downloads = root / "downloads"
    metadata = root / "metadata"
    extracted = root / "images"
    downloads.mkdir(parents=True, exist_ok=True)
    metadata.mkdir(parents=True, exist_ok=True)
    extracted.mkdir(parents=True, exist_ok=True)
    return downloads, metadata, extracted


def http_head_content_length(url: str) -> Optional[int]:
    req = urllib.request.Request(url, method="HEAD")
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            value = resp.headers.get("Content-Length")
            return int(value) if value and value.isdigit() else None
    except Exception:
        return None


def download_with_resume(url: str, dest: Path, force: bool = False) -> None:
    if force and dest.exists():
        dest.unlink()

    existing = dest.stat().st_size if dest.exists() else 0
    total = http_head_content_length(url)
    if total is not None and existing == total and existing > 0:
        log(f"[skip] {dest.name} already complete ({existing/1024/1024:.1f} MB)")
        return

    headers = {}
    mode = "wb"
    if existing > 0:
        headers["Range"] = f"bytes={existing}-"
        mode = "ab"

    req = urllib.request.Request(url, headers=headers)
    log(f"[download] {dest.name} start={existing} bytes")
    try:
        resp_ctx = urllib.request.urlopen(req, timeout=300)
    except HTTPError as e:
        # A 416 on ranged download usually means the local file is already complete.
        if e.code == 416 and existing > 0:
            log(f"[skip] {dest.name} range not satisfiable (HTTP 416), treating local file as complete")
            return
        raise

    with resp_ctx as resp, dest.open(mode) as f:
        if resp.status == 200 and existing > 0:
            # Server did not honor range; restart.
            f.close()
            dest.unlink(missing_ok=True)
            return download_with_resume(url, dest, force=False)

        downloaded = existing
        t0 = time.time()
        while True:
            chunk = resp.read(1024 * 1024)
            if not chunk:
                break
            f.write(chunk)
            downloaded += len(chunk)
            if total:
                pct = downloaded / total * 100.0
                log(f"  -> {dest.name}: {downloaded/1024/1024:.1f}/{total/1024/1024:.1f} MB ({pct:.1f}%)")
        dt = max(time.time() - t0, 1e-6)
        speed = (downloaded - existing) / dt / 1024 / 1024
        log(f"[done] {dest.name} +{(downloaded-existing)/1024/1024:.1f} MB @ {speed:.2f} MB/s")


def tar_quick_check(path: Path) -> bool:
    try:
        with tarfile.open(path, "r:gz") as tf:
            tf.getmembers()[:1]
        return True
    except Exception:
        return False


def extract_archive(path: Path, out_dir: Path) -> None:
    log(f"[extract] {path.name}")
    with tarfile.open(path, "r:gz") as tf:
        tf.extractall(out_dir)


def find_csv(metadata_dir: Path) -> Optional[Path]:
    cands = [
        metadata_dir / "Data_Entry_2017_v2020.csv",
        metadata_dir / "Data_Entry_2017.csv",
    ]
    for p in cands:
        if p.exists():
            return p
    return None


def collect_pngs(root: Path) -> Dict[str, Path]:
    files = [p for p in root.rglob("*.png") if p.is_file()]
    return {p.name: p for p in files}


def verify_integrity(root: Path, quick_tar_check: bool = True) -> Dict[str, object]:
    downloads, metadata, images_dir = ensure_dirs(root)
    csv_path = find_csv(metadata)

    report: Dict[str, object] = {
        "dataset_root": str(root),
        "downloads_dir": str(downloads),
        "metadata_dir": str(metadata),
        "images_dir": str(images_dir),
        "archives": [],
    }

    for i in range(1, 13):
        archive = downloads / infer_archive_name(i)
        item = {
            "name": archive.name,
            "exists": archive.exists(),
            "size_bytes": archive.stat().st_size if archive.exists() else 0,
        }
        if archive.exists() and quick_tar_check:
            item["tar_readable"] = tar_quick_check(archive)
        report["archives"].append(item)

    if not csv_path:
        report["csv_found"] = False
        report["error"] = "Data_Entry_2017*.csv not found in metadata directory"
        return report

    report["csv_found"] = True
    report["csv_path"] = str(csv_path)

    expected: List[str] = []
    with csv_path.open("r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            name = (row.get("Image Index") or "").strip()
            if name:
                expected.append(name)

    expected_set = set(expected)
    actual_map = collect_pngs(images_dir)
    actual_set = set(actual_map.keys())

    missing = sorted(expected_set - actual_set)
    orphans = sorted(actual_set - expected_set)
    report["csv_rows"] = len(expected)
    report["expected_unique_images"] = len(expected_set)
    report["actual_images"] = len(actual_set)
    report["missing_images"] = len(missing)
    report["missing_examples"] = missing[:200]
    report["orphan_images"] = len(orphans)
    report["orphan_examples"] = orphans[:200]
    report["coverage_ratio"] = (
        (len(expected_set) - len(missing)) / max(len(expected_set), 1)
    )
    report["csv_suspiciously_small"] = len(expected) < 1000
    if report["csv_suspiciously_small"]:
        report["warning"] = (
            "CSV rows are suspiciously small. You may have downloaded a sample metadata file "
            "instead of full NIH labels."
        )

    train_list = metadata / "train_val_list.txt"
    test_list = metadata / "test_list.txt"
    if train_list.exists():
        train_names = {x.strip() for x in train_list.read_text(encoding="utf-8").splitlines() if x.strip()}
        report["train_val_count"] = len(train_names)
        report["train_val_missing"] = len(train_names - actual_set)
    if test_list.exists():
        test_names = {x.strip() for x in test_list.read_text(encoding="utf-8").splitlines() if x.strip()}
        report["test_count"] = len(test_names)
        report["test_missing"] = len(test_names - actual_set)

    return report


def build_download_plan(root: Path) -> List[DownloadItem]:
    downloads, metadata, _ = ensure_dirs(root)
    plan: List[DownloadItem] = []
    for i, url in enumerate(IMAGE_URLS, start=1):
        plan.append(DownloadItem(url=url, path=downloads / infer_archive_name(i)))
    for name, url in METADATA_URLS.items():
        plan.append(DownloadItem(url=url, path=metadata / name))
    return plan


def main() -> None:
    parser = argparse.ArgumentParser(description="Download and verify NIH ChestXray14 dataset.")
    parser.add_argument("--dataset-root", required=True, help="Dataset root folder")
    parser.add_argument("--download", action="store_true", help="Download archives and metadata")
    parser.add_argument("--extract", action="store_true", help="Extract all downloaded archives")
    parser.add_argument("--verify", action="store_true", help="Run integrity checks")
    parser.add_argument("--force", action="store_true", help="Force re-download")
    parser.add_argument("--report", default="nih_integrity_report.json", help="Output report json path")
    args = parser.parse_args()

    root = Path(args.dataset_root).resolve()
    downloads, _, images_dir = ensure_dirs(root)

    if args.download:
        plan = build_download_plan(root)
        for item in plan:
            download_with_resume(item.url, item.path, force=args.force)

    if args.extract:
        for i in range(1, 13):
            archive = downloads / infer_archive_name(i)
            if not archive.exists():
                log(f"[skip] {archive.name} not found")
                continue
            extract_archive(archive, images_dir)

    if args.verify:
        report = verify_integrity(root)
        report_path = Path(args.report).resolve()
        report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
        log(f"[report] {report_path}")
        log(json.dumps(report, ensure_ascii=False, indent=2))

    if not args.download and not args.extract and not args.verify:
        parser.print_help()


if __name__ == "__main__":
    main()
