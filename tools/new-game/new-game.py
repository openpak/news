#!/usr/bin/env python3
"""Add a game's News channel: lineup entry, tile icon, welcome post, and an
"now available" post in openpak_games. Reusable for every new game.

  new-game.py --channels ../../channels --name "Mario Kart 8 Deluxe" \
      --slug mario_kart_8_deluxe --title-id 0100152000022000 --base-id 2610140151 \
      [--screenshot shot.jpg]

base-id: unique news_id; base+2 = welcome post, base+3 = Games post.
The tile icon comes from api.nlib.cc/nx/<title-id>/icon (override with --icon FILE).
Edit WELCOME / AVAILABLE below to change the wording for every game.
"""
import argparse, json, os, shutil, subprocess, sys

WELCOME = ("This channel carries news about {name} on OpenPak: server status, new features, fixes and events. "
           "Every post for this game lands here, so subscribe to keep up. Setup and troubleshooting live in "
           "OpenPak Help, and the full list of supported games is in OpenPak Games.")
AVAILABLE = ("{name} is now available on OpenPak. Follow its own channel, {name}, for news specific to this game, "
             "and check the Titles page for compatibility notes before you play online.")
FOOTER = "OpenPak Games — playable, not promised."

def article(i, title, body, footer, motif="pad", days=30):
    assert len(title) <= 100 and len(body) <= 1200 and len(footer) <= 80
    return {"news_id": i, "title": title, "body": body, "footer": footer, "language": "en-GB",
            "supported_languages": ["en-GB", "en-US"], "lifetime_days": days, "art": {"motif": motif}}

def dump(path, obj):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        json.dump(obj, f, indent=2, ensure_ascii=False); f.write("\n")

def main():
    a = argparse.ArgumentParser()
    a.add_argument("--channels", required=True); a.add_argument("--name", required=True)
    a.add_argument("--slug", required=True); a.add_argument("--title-id", required=True)
    a.add_argument("--base-id", type=int, required=True)
    a.add_argument("--screenshot"); a.add_argument("--icon")
    a = a.parse_args()
    topic, ch = "openpak_" + a.slug, a.channels
    assert len(a.slug) <= 23, "topic id is limited to 31 bytes"
    lp = os.path.join(ch, "lineup.json"); lineup = json.load(open(lp))
    if not any(c["topic_id"] == topic for c in lineup["channels"]):
        lineup["channels"].append({"topic_id": topic, "name": a.name[:100], "publisher": "OpenPak",
            "description": f"News about {a.name} on the OpenPak network."[:120], "important": False,
            "icon": {"motif": "mark-pad"}, "application_ids": [a.title_id.lower()]})
        dump(lp, lineup)
    icon = os.path.join(ch, "icons", topic + ".jpg"); os.makedirs(os.path.dirname(icon), exist_ok=True)
    src = a.icon or f"http://api.nlib.cc/nx/{a.title_id.upper()}/icon"
    tmp = icon + ".src"
    if a.icon: shutil.copy(a.icon, tmp)
    else: subprocess.run(["curl", "-fsSL", "-m", "60", "-o", tmp, src], check=True)
    subprocess.run(["magick", tmp, "-resize", "512x512^", "-gravity", "center", "-extent", "512x512",
                    "-quality", "90", icon], check=True); os.remove(tmp)
    w, g = a.base_id + 2, a.base_id + 3
    dump(f"{ch}/{topic}/{w}.json", article(w, f"Welcome to {a.name} on OpenPak", WELCOME.format(name=a.name), FOOTER))
    dump(f"{ch}/openpak_games/{g}.json", article(g, f"{a.name} is now available on OpenPak", AVAILABLE.format(name=a.name), FOOTER))
    if a.screenshot:
        for i in (w, g):
            shutil.copy(a.screenshot, f"{ch}/screenshots/{i}.jpg")
    print("ok", topic, w, g)

if __name__ == "__main__":
    sys.exit(main())
