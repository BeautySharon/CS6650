import json
import re
import sys
from typing import Dict

WORD_RE = re.compile(r"[a-zA-Z']+")


def count_words(text: str) -> Dict[str, int]:
    counts: Dict[str, int] = {}
    for m in WORD_RE.finditer(text.lower()):
        w = m.group(0)
        counts[w] = counts.get(w, 0) + 1
    return counts


def load_text(path: str) -> str:
    with open(path, "r", encoding="utf-8", errors="replace") as f:
        return f.read()


def load_json(path: str) -> Dict[str, int]:
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def main():
    if len(sys.argv) < 3:
        print("Usage: python baseline_verify.py <input.txt> <reducer_output.json> [num_samples]")
        sys.exit(1)

    input_path = sys.argv[1]
    out_path = sys.argv[2]
    num_samples = int(sys.argv[3]) if len(sys.argv) >= 4 else 20

    text = load_text(input_path)
    baseline = count_words(text)
    reduced = load_json(out_path)

    ok = True
    if len(baseline) != len(reduced):
        ok = False
        print(f"[Mismatch] unique_words baseline={len(baseline)} reducer={len(reduced)}")

    # Compare totals
    if sum(baseline.values()) != sum(reduced.values()):
        ok = False
        print(f"[Mismatch] total_words baseline={sum(baseline.values())} reducer={sum(reduced.values())}")

    # Sample check
    keys = list(baseline.keys())
    step = max(1, len(keys) // num_samples)
    samples = keys[::step][:num_samples]

    for w in samples:
        if baseline.get(w, 0) != int(reduced.get(w, 0)):
            ok = False
            print(f"[Mismatch] word='{w}' baseline={baseline.get(w)} reducer={reduced.get(w)}")

    if ok:
        print("[OK] Reducer output matches baseline under the same tokenization rules.")
    else:
        print("[FAIL] Found mismatches. Check tokenization consistency and file correctness.")


if __name__ == "__main__":
    main()
