#!/usr/bin/env python3
"""
build_translations.py — Combined translation builder for PoGoBot.

translations.json is rebuilt from scratch on every run from two sources:

  Step 0 — Masterfile fetch
      Downloads master-latest-react-map.json from the WatWowMap
      Masterfile-Generator repository and saves it as masterfile.json.

      Upstream source:
        https://raw.githubusercontent.com/WatWowMap/Masterfile-Generator
        /master/master-latest-react-map.json

  Step 1 — Game data (masterfile.json + pogo-translations)
      Maps Pokémon names, form names, move names, and item names from
      masterfile.json to translated names via poke_<id>, form_<id>,
      move_<id>, and item_<id> keys fetched from pogo-translations.
      Entries whose translation equals the English name are kept for Pokémon
      (many names are identical across languages) but dropped for all other
      game-data categories.

      Also maps team and raid level names directly from the locale source via
      team_<id> and raid_<id> keys (e.g. team_1 → "Weisheit", raid_5 →
      "Legendärer Raid" for German).  The _plural variants are skipped.

      Upstream source (fetched via HTTPS):
        https://raw.githubusercontent.com/WatWowMap/pogo-translations
        /master/static/locales/{lang}.json

  Step 1b — English canonical names (en.json)
      Always fetches en.json as well and writes an "en" section to
      translations.json containing the team_<id> and raid_<id> canonical
      English names (e.g. team_1 → "Mystic", raid_5 → "Legendary Raid").
      This is needed because the masterfile uses generic colour-based names
      ("Team Blue") rather than the canonical brand names, and because
      getTranslation() is no longer a no-op for English when these keys are
      involved.

  Step 2 — Bot UI strings (bot_strings.json)
      Merges application-specific bot UI strings — the literals used in
      Go tr.T("…") / tr.Tf("…") calls — from bot_strings.json[lang].
      These strings are maintained by hand and are not available from any
      upstream locale source.

      Also cross-checks bot_strings.json keys against the actual Go source
      and reports any drift (keys in Go but not in bot_strings.json, or
      keys in bot_strings.json that are no longer used in Go).

Usage:
  python build_translations.py [options]

Options:
  --masterfile FILE       Path to masterfile.json         (default: masterfile.json)
  --masterfile-url URL    Override URL to fetch masterfile from
                          (default: WatWowMap master-latest-react-map.json)
  --translations FILE     Path to translations.json       (default: translations.json)
  --bot-strings FILE      Path to bot_strings.json        (default: bot_strings.json)
  --lang CODE             Language code to build          (default: de)
  --dry-run               Print report only; do not write translations.json
  --check                 Exit with code 1 if any bot UI keys are missing
                          or if bot_strings.json is out of sync with Go source
                          (useful for CI)
"""

import argparse
import glob
import json
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path


# ── CLI ───────────────────────────────────────────────────────────────────────


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Build translations.json from masterfile + upstream locale sources.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    p.add_argument("--masterfile", default="masterfile.json", metavar="FILE")
    p.add_argument(
        "--masterfile-url",
        default=None,
        metavar="URL",
        dest="masterfile_url",
        help="Override URL to fetch masterfile from (default: WatWowMap master-latest-react-map.json)",
    )
    p.add_argument("--translations", default="translations.json", metavar="FILE")
    p.add_argument(
        "--bot-strings", default="bot_strings.json", metavar="FILE", dest="bot_strings"
    )
    p.add_argument("--lang", default="de", metavar="CODE")
    p.add_argument(
        "--dry-run",
        action="store_true",
        help="Print report only; do not write translations.json",
    )
    p.add_argument(
        "--check",
        action="store_true",
        help="Exit with code 1 if any bot UI translation keys are missing or bot_strings.json is out of sync",
    )
    return p.parse_args()


# ── File loading ──────────────────────────────────────────────────────────────


def load_json(path: str, label: str) -> dict:
    try:
        with open(path, encoding="utf-8") as f:
            return json.load(f)
    except FileNotFoundError:
        sys.exit(f"[ERROR] {label} not found: {path}")
    except json.JSONDecodeError as e:
        sys.exit(f"[ERROR] Failed to parse {path}: {e}")


# ── Upstream fetch ────────────────────────────────────────────────────────────

_POGO_TRANS_BASE = (
    "https://raw.githubusercontent.com/WatWowMap/pogo-translations"
    "/master/static/locales/{lang}.json"
)

_MASTERFILE_URL = (
    "https://raw.githubusercontent.com/WatWowMap/Masterfile-Generator"
    "/master/master-latest-react-map.json"
)


def fetch_json(url: str, label: str) -> dict:
    """Fetch JSON from *url* and return it as a dict.  Exits on network or parse error."""
    try:
        req = urllib.request.Request(
            url, headers={"User-Agent": "build_translations/1.0"}
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read()
        return json.loads(raw)
    except urllib.error.URLError as e:
        sys.exit(f"[ERROR] Failed to fetch {label}: {e}")
    except json.JSONDecodeError as e:
        sys.exit(f"[ERROR] Failed to parse {label} JSON: {e}")


def fetch_masterfile(url: str, dest_path: str) -> None:
    """
    Download the masterfile JSON from *url* and write it to *dest_path*,
    overwriting any existing file.  Exits on network or parse error.
    """
    data = fetch_json(url, "masterfile (master-latest-react-map.json)")
    with open(dest_path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=4)
    print(f"  Saved → {dest_path}")


def load_lang_source(lang: str) -> dict:
    """Fetch pogo-translations {lang}.json and return it."""
    print(f"\nFetching upstream locale source (pogo-translations/{lang}.json) …")
    return fetch_json(
        _POGO_TRANS_BASE.format(lang=lang), f"pogo-translations {lang}.json"
    )


# ── Step 1: Game data ─────────────────────────────────────────────────────────


def build_game_data_translations(
    masterfile: dict,
    lang_src: dict,
) -> tuple[dict, list[str]]:
    """
    Return (entries, still_missing) where:
      entries        — {english_name: translated_name} for every masterfile name
                       that has a translation in lang_src, including entries
                       where the translation is identical to the English name
                       (kept to avoid spurious "Translation key not found" logs)
      still_missing  — english names present in masterfile but without a
                       translation in lang_src
    """
    poke_map: dict[str, str] = {}
    form_map: dict[str, str] = {}
    move_map: dict[str, str] = {}
    item_map: dict[str, str] = {}

    for key, value in lang_src.items():
        if key.startswith("poke_") and "_e" not in key:
            poke_map[key[5:]] = value
        elif key.startswith("form_"):
            form_map[key[5:]] = value
        elif key.startswith("move_"):
            move_map[key[5:]] = value
        elif key.startswith("item_"):
            item_map[key[5:]] = value

    entries: dict[str, str] = {}
    needed: set[str] = set()

    def _add(en_name: str, translated: str | None) -> None:
        if not en_name:
            return
        needed.add(en_name)
        if not translated:
            return
        entries[en_name] = translated

    for pid, pokemon in masterfile.get("pokemon", {}).items():
        en_name = pokemon.get("name", "")
        _add(en_name, poke_map.get(str(pid)))
        for fid, form in pokemon.get("forms", {}).items():
            form_name = form.get("name", "")
            _add(form_name, form_map.get(str(fid)))

    for mid, move in masterfile.get("moves", {}).items():
        en_name = move.get("name", "")
        _add(en_name, move_map.get(str(mid)))

    for iid, item in masterfile.get("items", {}).items():
        en_name = item if isinstance(item, str) else item.get("name", "")
        _add(en_name, item_map.get(str(iid)))

    still_missing = sorted(needed - set(entries))
    return entries, still_missing


# ── Step 1b: Team and raid names ──────────────────────────────────────────────


def build_team_raid_translations(lang_src: dict) -> dict:
    """
    Return {key: translated_name} for all team_<id> and raid_<id> entries in
    lang_src, skipping the _plural variants.  The keys are kept as-is (e.g.
    "team_1", "raid_5") so the Go Translator can look them up directly by
    numeric ID without going through the masterfile's generic English strings.
    """
    entries: dict[str, str] = {}
    for key, value in lang_src.items():
        if key.startswith("team_") and not key.endswith("_plural"):
            # Skip the "team_a_<id>" variant (full "Team Mystic" form); we only
            # want the short names (team_1 → "Mystic" / "Weisheit").
            parts = key.split("_")
            if len(parts) == 2:
                entries[key] = value
        elif key.startswith("raid_") and not key.endswith("_plural"):
            entries[key] = value
    return entries


# ── Step 2: Bot UI strings ────────────────────────────────────────────────────

# Matches any .T("…") or .Tf("…") call — covers both:
#   tr.T("…") / tr.Tf("…")          — direct translator variable
#   newTranslatorFor(c).T("…")       — inline construction + call
# Handles simple \" and \\ escapes inside the string literal.
_GO_TR_RE = re.compile(r'\.Tf?\(\s*"((?:[^"\\]|\\.)*)"')


def scan_go_keys(project_root: str) -> list[str]:
    """
    Return a sorted, deduplicated list of string literals passed as the first
    argument to any .T() or .Tf() call in any *.go file inside project_root
    (non-recursive — only the immediate directory).
    """
    keys: set[str] = set()
    pattern = str(Path(project_root) / "*.go")
    for filepath in glob.glob(pattern):
        try:
            text = Path(filepath).read_text(encoding="utf-8")
        except OSError:
            continue
        for match in _GO_TR_RE.finditer(text):
            raw = match.group(1)
            keys.add(raw.replace('\\"', '"').replace("\\\\", "\\"))
    return sorted(keys)


def load_bot_strings(bot_strings_file: str, lang: str) -> dict[str, str]:
    """
    Load bot_strings.json and return the dict for *lang*.
    Returns an empty dict if the language section is absent.
    """
    data = load_json(bot_strings_file, "bot_strings.json")
    return dict(data.get(lang, {}))


# ── Reporting ─────────────────────────────────────────────────────────────────


def _section(title: str) -> None:
    print(f"\n{'─' * 60}")
    print(f"  {title}")
    print(f"{'─' * 60}")


def print_report(
    lang: str,
    step1_entries: dict,
    step1_missing: list[str],
    step1b_entries: dict,
    step1b_en_entries: dict,
    step2_entries: dict,
    final_map: dict,
    dry_run: bool,
) -> None:
    """Print a human-readable summary."""
    print("\n╔══════════════════════════════════════════════════════════╗")
    print("║             build_translations.py — Report               ║")
    print("╚══════════════════════════════════════════════════════════╝")

    _section("Step 1 — Game data (Pokémon / forms / moves / items)")
    print(f"  Entries built     : {len(step1_entries)}")
    if step1_missing:
        print(f"  Still missing     : {len(step1_missing)}")
        for name in step1_missing:
            print(f"    • {name}")
    else:
        print("  Still missing     : 0 ✅")

    _section(f"Step 1b — Team / raid names ({lang}.json + en.json)")
    print(f"  {lang} entries : {len(step1b_entries)}")
    print(f"  en entries   : {len(step1b_en_entries)}")

    _section("Step 2 — Bot UI strings (bot_strings.json)")
    print(f"  Entries in bot_strings.json[{lang!r}] : {len(step2_entries)}")

    _section("Summary")
    print(f"  Total entries in translations.json[{lang!r}] : {len(final_map)}")
    if dry_run:
        print("\n  [DRY RUN] translations.json was NOT modified.")
    else:
        print("\n  translations.json updated successfully.")


def check_bot_strings(
    lang: str,
    step2_entries: dict,
    go_keys: list[str],
    final_map: dict,
) -> int:
    """
    Cross-check bot_strings.json against Go source keys.
    Prints findings and returns the number of problems (for --check exit code).
    """
    bot_keys_set = set(step2_entries)
    go_keys_set = set(go_keys)

    in_go_not_in_bot = sorted(go_keys_set - bot_keys_set)
    in_bot_not_in_go = sorted(bot_keys_set - go_keys_set)
    missing_translations = [k for k in go_keys if k not in final_map]

    _section("Go source cross-check")
    print(f"  Entries in bot_strings.json[{lang!r}] : {len(step2_entries)}")
    print(f"  Go source keys found              : {len(go_keys)}")

    if in_go_not_in_bot:
        print(
            f"  ⚠️  In Go source but missing from bot_strings.json ({len(in_go_not_in_bot)}):"
        )
        for k in in_go_not_in_bot:
            print(f"    ❌  {k!r}")
    else:
        print("  In Go source, missing from bot_strings.json : 0 ✅")

    if in_bot_not_in_go:
        print(
            f"  ⚠️  In bot_strings.json but not found in Go source ({len(in_bot_not_in_go)}):"
        )
        for k in in_bot_not_in_go:
            print(f"    ⚠️   {k!r}")
    else:
        print("  In bot_strings.json, not in Go source : 0 ✅")

    if missing_translations:
        print(f"  Missing translation for ({len(missing_translations)}) Go keys:")
        for k in missing_translations:
            print(f"    ❌  {k!r}")
    else:
        print("  Bot UI translations covered : 100% ✅")

    return len(in_go_not_in_bot) + len(missing_translations)


# ── Main ──────────────────────────────────────────────────────────────────────


def main() -> None:
    args = parse_args()

    # ── Step 0: Fetch masterfile ──────────────────────────────────────────────
    mf_url = args.masterfile_url if args.masterfile_url else _MASTERFILE_URL
    print("\nFetching upstream masterfile …")
    fetch_masterfile(mf_url, args.masterfile)

    # ── Load inputs ───────────────────────────────────────────────────────────
    masterfile = load_json(args.masterfile, "masterfile.json")
    lang_src = load_lang_source(args.lang)

    lang = args.lang

    # ── Step 1 — Game data ────────────────────────────────────────────────────
    step1_entries, step1_missing = build_game_data_translations(masterfile, lang_src)

    # ── Step 1b — Team / raid names ───────────────────────────────────────────
    # Always fetch en.json alongside the target language so that English users
    # get canonical brand names (Mystic / Valor / Instinct, Legendary Raid, …)
    # rather than the generic masterfile strings (Team Blue, Raid Level 5, …).
    step1b_entries = build_team_raid_translations(lang_src)
    en_src = load_lang_source("en") if lang != "en" else lang_src
    step1b_en_entries = build_team_raid_translations(en_src)

    # ── Step 2 — Bot UI strings ───────────────────────────────────────────────
    step2_entries = load_bot_strings(args.bot_strings, lang)

    # ── Build final maps ──────────────────────────────────────────────────────
    # For the target language: game data + team/raid names + bot strings.
    # bot_strings.json takes precedence over game data for any overlapping key.
    final_map: dict[str, str] = {**step1_entries, **step1b_entries, **step2_entries}

    # For English: only the team/raid canonical names are needed; everything
    # else falls back to the key itself (getTranslation is still a no-op for
    # regular string keys when lang=="en").
    en_map: dict[str, str] = step1b_en_entries

    # ── Report ────────────────────────────────────────────────────────────────
    print_report(
        lang,
        step1_entries,
        step1_missing,
        step1b_entries,
        step1b_en_entries,
        step2_entries,
        final_map,
        args.dry_run,
    )

    # ── Write ─────────────────────────────────────────────────────────────────
    if not args.dry_run:
        output: dict[str, dict[str, str]] = {
            lang: dict(final_map.items()),
        }
        if lang != "en":
            output["en"] = dict(en_map.items())
        with open(args.translations, "w", encoding="utf-8") as f:
            json.dump(output, f, ensure_ascii=False, indent=4)

    # ── Check (requires Go source) ────────────────────────────────────────────
    if args.check:
        project_root = str(Path(args.translations).parent)
        go_keys = scan_go_keys(project_root)
        problem_count = check_bot_strings(lang, step2_entries, go_keys, final_map)
        if problem_count > 0:
            sys.exit(1)


if __name__ == "__main__":
    main()
