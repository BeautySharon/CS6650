import os
import time
import json
from typing import Dict, Tuple, List
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


def s3_read_json(s3_url: str) -> Dict[str, int]:
    bucket, key = parse_s3_url(s3_url)
    obj = s3.get_object(Bucket=bucket, Key=key)
    data = obj["Body"].read().decode("utf-8", errors="replace")
    return json.loads(data)


def s3_write_json(bucket: str, key: str, obj: Dict) -> str:
    body = json.dumps(obj, ensure_ascii=False, sort_keys=True).encode("utf-8")
    s3.put_object(
        Bucket=bucket,
        Key=key,
        Body=body,
        ContentType="application/json; charset=utf-8",
    )
    return f"s3://{bucket}/{key}"


def merge_counts(target: Dict[str, int], part: Dict[str, int]) -> None:
    for w, c in part.items():
        target[w] = target.get(w, 0) + int(c)


@app.get("/health")
def health():
    return jsonify({"ok": True})


@app.post("/reduce")
def reduce_maps():
    """
    Body:
    {
      "map_outputs": ["s3://.../maps/a.json", "s3://.../maps/b.json", ...],
      "output_s3": "s3://BUCKET/results/final.json"
    }
    """
    t0 = time.time()
    body = request.get_json(force=True)

    map_outputs: List[str] = body.get("map_outputs", [])
    output_s3: str = body.get("output_s3")

    if not map_outputs or not isinstance(map_outputs, list):
        return jsonify({"error": "map_outputs must be a non-empty list"}), 400
    if not output_s3:
        return jsonify({"error": "missing output_s3"}), 400

    out_bucket, out_key = parse_s3_url(output_s3)

    final: Dict[str, int] = {}
    for url in map_outputs:
        part = s3_read_json(url)
        merge_counts(final, part)

    total_words = sum(final.values())
    elapsed_ms = int((time.time() - t0) * 1000)

    out_url = s3_write_json(out_bucket, out_key, final)
    return jsonify({
        "result": out_url,
        "unique_words": len(final),
        "total_words": total_words,
        "elapsed_ms": elapsed_ms
    })

if __name__ == "__main__":
    # Important: bind to 0.0.0.0 so ECS can expose the port to the internet
    app.run(host="0.0.0.0", port=int(os.getenv("PORT", "5000")))