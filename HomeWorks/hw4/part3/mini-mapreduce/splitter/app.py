import os
import re
import time
import json
from typing import Tuple, List
from urllib.parse import urlparse

import boto3
from flask import Flask, request, jsonify

app = Flask(__name__)

AWS_REGION = os.getenv("AWS_REGION", "us-east-1")
s3 = boto3.client("s3", region_name=AWS_REGION)


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


def s3_write_text(bucket: str, key: str, text: str) -> str:
    s3.put_object(
        Bucket=bucket,
        Key=key,
        Body=text.encode("utf-8"),
        ContentType="text/plain; charset=utf-8",
    )
    return f"s3://{bucket}/{key}"


@app.get("/health")
def health():
    return jsonify({"ok": True})


@app.post("/split")
def split():
    """
    Body:
    {
      "input_s3": "s3://BUCKET/input/file.txt",
      "num_chunks": 3,
      "chunk_prefix": "chunks/"
    }
    """
    t0 = time.time()
    body = request.get_json(force=True)

    input_s3 = body.get("input_s3")
    num_chunks = int(body.get("num_chunks", 3))
    chunk_prefix = body.get("chunk_prefix", "chunks/")

    if not input_s3:
        return jsonify({"error": "missing input_s3"}), 400
    if num_chunks <= 0:
        return jsonify({"error": "num_chunks must be > 0"}), 400

    in_bucket, in_key = parse_s3_url(input_s3)
    text = s3_read_text(input_s3)

    # Split by lines to avoid breaking words.
    lines = text.splitlines(True)  # keep line breaks
    total_lines = len(lines)

    # If file is tiny, still create num_chunks outputs (some may be empty).
    base = total_lines // num_chunks
    rem = total_lines % num_chunks

    chunks: List[str] = []
    start = 0
    base_name = os.path.basename(in_key)
    # Keep same "stem" if possible
    stem = base_name
    for i in range(num_chunks):
        take = base + (1 if i < rem else 0)
        part_lines = lines[start:start + take]
        start += take

        part_key = f"{chunk_prefix.rstrip('/')}/{stem}.part{i}.txt"
        part_text = "".join(part_lines)

        out_url = s3_write_text(in_bucket, part_key, part_text)
        chunks.append(out_url)

    elapsed_ms = int((time.time() - t0) * 1000)
    return jsonify({
        "input": input_s3,
        "num_chunks": num_chunks,
        "chunks": chunks,
        "elapsed_ms": elapsed_ms
    })

if __name__ == "__main__":
    # Important: bind to 0.0.0.0 so ECS can expose the port to the internet
    app.run(host="0.0.0.0", port=int(os.getenv("PORT", "5000")))