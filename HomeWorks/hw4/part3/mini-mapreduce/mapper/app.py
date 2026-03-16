import os
import re
import time
import json
from typing import Dict, Tuple
from urllib.parse import urlparse

import boto3
from flask import Flask, request, jsonify

app = Flask(__name__)

AWS_REGION = os.getenv("AWS_REGION", "us-east-1")
s3 = boto3.client("s3", region_name=AWS_REGION)

WORD_RE = re.compile(r"[a-zA-Z']+")


def parse_s3_url(s3_url: str) -> Tuple[str, str]:
    if not s3_url.startswith("s3://"):
        raise ValueError(f"Invalid s3 url (must start with s3://): {s3_url}")
    u = urlparse(s3_url)
    bucket = u.netloc
    key = u.path.lstrip("/")
    if not bucket or not key:
        raise ValueError(f"Invalid s3 url: {s3_url}")
    return bucket, key


def s3_read_text(s3_url: str) -> str:
    bucket, key = parse_s3_url(s3_url)
    obj = s3.get_object(Bucket=bucket, Key=key)
    data = obj["Body"].read()
    return data.decode("utf-8", errors="replace")


def s3_write_json(bucket: str, key: str, obj: Dict) -> str:
    body = json.dumps(obj, ensure_ascii=False, sort_keys=True).encode("utf-8")
    s3.put_object(
        Bucket=bucket,
        Key=key,
        Body=body,
        ContentType="application/json; charset=utf-8",
    )
    return f"s3://{bucket}/{key}"


def count_words(text: str) -> Dict[str, int]:
    counts: Dict[str, int] = {}
    for m in WORD_RE.finditer(text.lower()):
        w = m.group(0)
        counts[w] = counts.get(w, 0) + 1
    return counts


@app.get("/health")
def health():
    return jsonify({"ok": True})


@app.post("/map")
def map_chunk():
    """
    Body:
    {
      "chunk_s3": "s3://BUCKET/chunks/file.part0.txt",
      "output_prefix": "maps/"
    }
    """
    t0 = time.time()
    body = request.get_json(force=True)

    chunk_s3 = body.get("chunk_s3")
    output_prefix = body.get("output_prefix", "maps/")

    if not chunk_s3:
        return jsonify({"error": "missing chunk_s3"}), 400

    bucket, chunk_key = parse_s3_url(chunk_s3)
    text = s3_read_text(chunk_s3)
    counts = count_words(text)

    base_name = os.path.basename(chunk_key)
    out_key = f"{output_prefix.rstrip('/')}/{base_name}.json"
    out_url = s3_write_json(bucket, out_key, counts)

    elapsed_ms = int((time.time() - t0) * 1000)
    return jsonify({
        "chunk": chunk_s3,
        "map_out": out_url,
        "unique_words": len(counts),
        "elapsed_ms": elapsed_ms
    })

if __name__ == "__main__":
    # Important: bind to 0.0.0.0 so ECS can expose the port to the internet
    app.run(host="0.0.0.0", port=int(os.getenv("PORT", "5000")))