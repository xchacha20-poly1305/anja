#!/usr/bin/env python3

import argparse
import os
import re
from pathlib import Path


PKG_ORIGINAL = "github.com/sagernet/gomobile"
PKG_NEW = "github.com/xchacha20-poly1305/anja"

TEXT_RULES = [
    {
        "forward_pattern": re.escape(PKG_ORIGINAL),
        "forward_replacement": PKG_NEW,
        "reverse_pattern": re.escape(PKG_NEW),
        "reverse_replacement": PKG_ORIGINAL,
    },
    {
        "forward_pattern": r"(\\[abfnrtv0-9xuUxX]+)gomobile(?![A-Za-z0-9])",
        "forward_replacement": r"\1anja",
        "reverse_pattern": r"(\\[abfnrtv0-9xuUxX]+)anja(?![A-Za-z0-9])",
        "reverse_replacement": r"\1gomobile",
    },
    {
        "forward_pattern": r"(\\[abfnrtv0-9xuUxX]+)gobind(?![A-Za-z0-9])",
        "forward_replacement": r"\1anjb",
        "reverse_pattern": r"(\\[abfnrtv0-9xuUxX]+)anjb(?![A-Za-z0-9])",
        "reverse_replacement": r"\1gobind",
    },
    {
        "forward_pattern": r"(?<![A-Za-z0-9])gomobile(?![A-Za-z0-9])",
        "forward_replacement": "anja",
        "reverse_pattern": r"(?<![A-Za-z0-9])anja(?![A-Za-z0-9])",
        "reverse_replacement": "gomobile",
    },
    {
        "forward_pattern": r"(?<![A-Za-z0-9])gobind(?![A-Za-z0-9])",
        "forward_replacement": "anjb",
        "reverse_pattern": r"(?<![A-Za-z0-9])anjb(?![A-Za-z0-9])",
        "reverse_replacement": "gobind",
    },
    {
        "forward_pattern": r"(?<![A-Za-z0-9])Gomobile(?![A-Za-z0-9])",
        "forward_replacement": "Anja",
        "reverse_pattern": r"(?<![A-Za-z0-9])Anja(?![A-Za-z0-9])",
        "reverse_replacement": "Gomobile",
    },
    {
        "forward_pattern": r"(?<![A-Za-z0-9])Gobind(?![A-Za-z0-9])",
        "forward_replacement": "Anjb",
        "reverse_pattern": r"(?<![A-Za-z0-9])Anjb(?![A-Za-z0-9])",
        "reverse_replacement": "Gobind",
    },
]

FORWARD_DIR_RULES = [
    ("cmd/gomobile", "cmd/anja"),
    ("cmd/gobind", "cmd/anjb"),
]


def build_rules(reverse):
    if not reverse:
        text_rules = [
            (rule["forward_pattern"], rule["forward_replacement"]) for rule in TEXT_RULES
        ]
        return text_rules, FORWARD_DIR_RULES

    reverse_text = [
        (rule["reverse_pattern"], rule["reverse_replacement"]) for rule in TEXT_RULES
    ]
    reverse_dirs = [(dst, src) for src, dst in FORWARD_DIR_RULES]
    return reverse_text, reverse_dirs


def rename_dirs(root, dir_rules, dry_run):
    renamed = 0
    for src_rel, dst_rel in dir_rules:
        src = root / src_rel
        dst = root / dst_rel

        if not src.exists():
            continue
        if dst.exists():
            raise RuntimeError(
                "cannot rename directory, destination exists: {} -> {}".format(
                    src, dst
                )
            )

        print("rename dir: {} -> {}".format(src, dst))
        if not dry_run:
            src.rename(dst)
        renamed += 1

    return renamed


def replace_in_files(root, compiled_rules, dry_run, script_path):
    changed_files = 0
    replacements = 0
    skipped_binary = 0

    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if not d.startswith(".")]
        for filename in filenames:
            path = Path(dirpath) / filename
            if path.resolve() == script_path:
                continue

            data = path.read_bytes()
            if b"\x00" in data:
                skipped_binary += 1
                continue
            try:
                text = data.decode("utf-8")
            except UnicodeDecodeError:
                skipped_binary += 1
                continue

            new_text = text
            file_replacements = 0
            for pattern, replacement in compiled_rules:
                new_text, n = pattern.subn(replacement, new_text)
                file_replacements += n

            if file_replacements == 0:
                continue

            print("update file: {} ({} replacements)".format(path, file_replacements))
            if not dry_run:
                path.write_text(new_text, encoding="utf-8")
            changed_files += 1
            replacements += file_replacements

    return changed_files, replacements, skipped_binary


def main():
    parser = argparse.ArgumentParser(
        description="Rename gomobile/gobind project to anja/anjb."
    )
    parser.add_argument("-r", "--reverse", action="store_true")
    parser.add_argument("-n", "--dry-run", action="store_true")
    parser.add_argument("--root", default=".", help="project root to process")
    args = parser.parse_args()

    root = Path(args.root).resolve()
    script_path = Path(__file__).resolve()

    text_rules, dir_rules = build_rules(args.reverse)
    compiled_rules = [
        (re.compile(pattern), replacement) for pattern, replacement in text_rules
    ]

    renamed_dirs = rename_dirs(root, dir_rules, args.dry_run)
    changed_files, replacements, skipped_binary = replace_in_files(
        root, compiled_rules, args.dry_run, script_path
    )

    print(
        "done: {} dirs renamed, {} files changed, {} replacements, {} binary files skipped".format(
            renamed_dirs, changed_files, replacements, skipped_binary
        )
    )


if __name__ == "__main__":
    main()
