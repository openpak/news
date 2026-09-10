#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Build original decoded samples only; never publish or access credentials."""
import argparse
import datetime as dt
import json
from pathlib import Path
import subprocess

import msgpack


def graphic(width, height):
    pixels = bytearray()
    for y in range(height):
        for x in range(width):
            color = (17, 28, 47)
            if width // 8 < x < width * 7 // 8 and height // 3 < y < height * 2 // 3:
                color = (43, 212, 158)
            if width * 3 // 8 < x < width * 5 // 8 and height // 8 < y < height * 7 // 8:
                color = (55, 136, 240)
            pixels.extend(color)
    ppm = f"P6\n{width} {height}\n255\n".encode() + pixels
    return subprocess.run(
        ["cjpeg", "-quality", "85", "-baseline", "-sample", "1x1"],
        input=ppm, stdout=subprocess.PIPE, check=True,
    ).stdout


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, default=Path(__file__).parent)
    parser.add_argument("--out", type=Path, required=True, help="new output directory")
    parser.add_argument("--published", help="ISO 8601 UTC timestamp; default now")
    args = parser.parse_args()
    channel = json.loads((args.source / "channel.json").read_text())
    article = json.loads((args.source / "article.json").read_text())
    when = dt.datetime.fromisoformat(args.published) if args.published else dt.datetime.now(dt.timezone.utc)
    if when.tzinfo is None:
        parser.error("--published must include a timezone")
    if not isinstance(article["news_id"], int) or isinstance(article["news_id"], bool) or article["news_id"] <= 0:
        parser.error("news_id must be a positive integer")
    if not isinstance(article["lifetime_days"], int) or not 1 <= article["lifetime_days"] <= 30:
        parser.error("lifetime_days must be 1..30")
    published = int(when.timestamp())
    expires = published + article["lifetime_days"] * 86400
    icon, card, hero = graphic(70, 70), graphic(358, 201), graphic(800, 364)
    record = dict(channel, publishing_time=published, last_posted_at=published)
    payload = {
        "version": {"format": 1, "semantics": 1},
        "news_id": article["news_id"], "published_at": published, "expire_at": expires,
        "pickup_limit": expires, "priority": 50, "deletion_priority": 100,
        "language": article["language"], "supported_languages": article["supported_languages"],
        "display_type": "NORMAL", "topic_id": channel["topic_id"],
        "no_photography": 0, "surprise": 0, "essential_pickup_limit": 0, "movie": 0,
        "subject": {"caption": 1, "text": article["title"]},
        "topic_name": channel["name"], "topic_image": icon,
        "topic_publisher": channel["publisher"], "topic_description": channel["description"],
        "list_image": card, "footer": {"text": article["footer"]},
        "body": {"text": article["body"], "main_image_height": 364, "main_image": hero},
    }
    values = {
        "catalog.msgpack": [record],
        "detail.msgpack": dict(record, latest_news_urls=[]),
        "archives.msgpack": {"na_required": False, "data_list": []},
        "article.msgpack": payload,
        "article-expired.msgpack": dict(payload, published_at=published - 86400,
                                        expire_at=published - 1, pickup_limit=published - 1),
    }
    args.out.mkdir(parents=True, exist_ok=False)
    for name, value in values.items():
        (args.out / name).write_bytes(msgpack.packb(value, use_bin_type=True))
    for name, data in [("icon.jpg", icon), ("card.jpg", card), ("hero.jpg", hero)]:
        (args.out / name).write_bytes(data)
    print(f"Created decoded samples in {args.out}; no content published.")
    print("Archive list is intentionally empty; article delivery wiring is unimplemented.")


if __name__ == "__main__":
    main()
