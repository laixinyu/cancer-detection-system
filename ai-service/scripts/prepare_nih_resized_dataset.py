#!/usr/bin/env python3
import argparse
import csv
import json
import os
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Dict, List, Tuple

from PIL import Image


def resolve_csv_path(dataset_root: Path) -> Path:
    direct = [
        dataset_root / "metadata" / "Data_Entry_2017_v2020.csv",
        dataset_root / "metadata" / "Data_Entry_2017.csv",
        dataset_root / "Data_Entry_2017_v2020.csv",
        dataset_root / "Data_Entry_2017.csv",
    ]
    for p in direct:
        if p.exists():
            return p
    found = sorted(dataset_root.rglob("Data_Entry_2017*.csv"))
    if found:
        return found[0]
    raise FileNotFoundError("Data_Entry_2017*.csv not found")


def resolve_list_path(dataset_root: Path, name: str) -> Path:
    direct = [
        dataset_root / "metadata" / name,
        dataset_root / name,
    ]
    for p in direct:
        if p.exists():
            return p
    found = sorted(dataset_root.rglob(name))
    return found[0] if found else Path("")


def collect_images(dataset_root: Path) -> Dict[str, Path]:
    image_map: Dict[str, Path] = {}
    for p in dataset_root.rglob("*"):
        if not p.is_file():
            continue
        if p.suffix.lower() not in {".png", ".jpg", ".jpeg"}:
            continue
        image_map[p.name] = p
    return image_map


def convert_one(src: Path, dst: Path, size: int, quality: int, skip_existing: bool) -> Tuple[bool, str]:
    if skip_existing and dst.exists():
        return True, ""
    dst.parent.mkdir(parents=True, exist_ok=True)
    try:
        with Image.open(src) as im:
            out = im.convert("L").resize((size, size), Image.Resampling.BILINEAR)
            out.save(dst, format="JPEG", quality=quality, optimize=True)
        return True, ""
    except Exception as exc:
        return False, f"{src.name}: {exc}"


def rewrite_list(src: Path, dst: Path, rename_map: Dict[str, str]) -> int:
    if not src.exists():
        return 0
    lines = [x.strip() for x in src.read_text(encoding="utf-8").splitlines() if x.strip()]
    out = [rename_map.get(name, f"{Path(name).stem}.jpg") for name in lines]
    dst.parent.mkdir(parents=True, exist_ok=True)
    dst.write_text("\n".join(out) + "\n", encoding="utf-8")
    return len(out)


def main() -> None:
    parser = argparse.ArgumentParser(description="Build resized NIH dataset with rewritten metadata.")
    parser.add_argument("--dataset-root", required=True, help="Original NIH dataset root")
    parser.add_argument("--output-root", required=True, help="Output dataset root")
    parser.add_argument("--size", type=int, default=512, help="Square output size, e.g. 224 or 512")
    parser.add_argument("--quality", type=int, default=90, help="JPEG quality")
    parser.add_argument("--workers", type=int, default=min(16, max(4, (os.cpu_count() or 8) - 2)))
    parser.add_argument("--skip-existing", action="store_true", default=True)
    args = parser.parse_args()

    dataset_root = Path(args.dataset_root).resolve()
    output_root = Path(args.output_root).resolve()
    out_images = output_root / "images"
    out_meta = output_root / "metadata"
    out_meta.mkdir(parents=True, exist_ok=True)

    csv_path = resolve_csv_path(dataset_root)
    train_val_path = resolve_list_path(dataset_root, "train_val_list.txt")
    test_path = resolve_list_path(dataset_root, "test_list.txt")

    image_map = collect_images(dataset_root)
    print(f"[info] source images indexed: {len(image_map)}")
    print(f"[info] csv: {csv_path}")

    rows: List[dict] = []
    rename_map: Dict[str, str] = {}
    tasks: List[Tuple[Path, Path]] = []
    with csv_path.open("r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            old_name = (row.get("Image Index") or "").strip()
            if not old_name:
                continue
            src = image_map.get(old_name)
            if src is None:
                continue
            new_name = f"{Path(old_name).stem}.jpg"
            rename_map[old_name] = new_name
            row["Image Index"] = new_name
            rows.append(row)
            tasks.append((src, out_images / new_name))

    ok_count = 0
    fail_count = 0
    failures: List[str] = []
    total = len(tasks)
    print(f"[info] convert tasks: {total}, workers={args.workers}, size={args.size}")
    with ThreadPoolExecutor(max_workers=args.workers) as ex:
        futs = [ex.submit(convert_one, src, dst, args.size, args.quality, args.skip_existing) for src, dst in tasks]
        for i, fut in enumerate(as_completed(futs), 1):
            ok, err = fut.result()
            if ok:
                ok_count += 1
            else:
                fail_count += 1
                if len(failures) < 200:
                    failures.append(err)
            if i % 2000 == 0 or i == total:
                print(f"[progress] {i}/{total} done")

    out_csv = out_meta / csv_path.name
    with out_csv.open("w", encoding="utf-8", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=rows[0].keys() if rows else [])
        if rows:
            writer.writeheader()
            writer.writerows(rows)

    train_val_count = rewrite_list(train_val_path, out_meta / "train_val_list.txt", rename_map) if train_val_path else 0
    test_count = rewrite_list(test_path, out_meta / "test_list.txt", rename_map) if test_path else 0

    report = {
        "dataset_root": str(dataset_root),
        "output_root": str(output_root),
        "size": args.size,
        "quality": args.quality,
        "workers": args.workers,
        "csv_in": str(csv_path),
        "csv_out": str(out_csv),
        "images_total": total,
        "images_ok": ok_count,
        "images_failed": fail_count,
        "train_val_count": train_val_count,
        "test_count": test_count,
        "failures": failures,
    }
    report_path = output_root / f"resize_report_{args.size}.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"[done] report: {report_path}")
    print(f"[done] images_ok={ok_count}, images_failed={fail_count}")


if __name__ == "__main__":
    main()
