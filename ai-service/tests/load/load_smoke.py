import argparse
import asyncio
import time
from pathlib import Path

import aiohttp


async def one_call(session: aiohttp.ClientSession, url: str, image_path: Path, api_key: str | None):
    headers = {}
    if api_key:
        headers["x-api-key"] = api_key
    data = aiohttp.FormData()
    data.add_field("file", image_path.read_bytes(), filename=image_path.name, content_type="image/jpeg")
    start = time.perf_counter()
    async with session.post(url, data=data, headers=headers) as resp:
        await resp.read()
        return resp.status, time.perf_counter() - start


async def run(args):
    async with aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=args.timeout)) as session:
        tasks = [one_call(session, args.url, Path(args.image), args.api_key) for _ in range(args.n)]
        results = await asyncio.gather(*tasks, return_exceptions=True)

    ok = [r for r in results if isinstance(r, tuple)]
    errors = [r for r in results if not isinstance(r, tuple)]
    if ok:
        lat = [x[1] for x in ok]
        print(f"count={len(ok)} errors={len(errors)} min={min(lat):.3f}s max={max(lat):.3f}s avg={sum(lat)/len(lat):.3f}s")
        by_status = {}
        for status, _ in ok:
            by_status[status] = by_status.get(status, 0) + 1
        print(f"status={by_status}")
    else:
        print("no successful responses")
    if errors:
        print(f"exceptions={len(errors)}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="AI service load smoke test")
    parser.add_argument("--url", default="http://127.0.0.1:8000/predict")
    parser.add_argument("--image", required=True)
    parser.add_argument("--n", type=int, default=20)
    parser.add_argument("--timeout", type=float, default=30)
    parser.add_argument("--api-key", default="")
    args = parser.parse_args()
    asyncio.run(run(args))
