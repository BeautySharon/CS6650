from locust import HttpUser, task, between, events
import random
import os
import csv
import time
from collections import defaultdict
from threading import Lock

# -----------------------------
# Hotspot keys: force temporal locality
# -----------------------------
KEYS = ["k1", "k2", "k3", "k4", "k5"]

# Write ratio from environment variable
# Example:
# WRITE_RATIO=0.1 locust ...
WRITE_RATIO = float(os.getenv("WRITE_RATIO", "0.1"))

# Output prefix for custom files
RESULT_PREFIX = os.getenv("RESULT_PREFIX", f"results_{WRITE_RATIO}")

# -----------------------------
# Shared tracking state
# -----------------------------
latest_versions = {k: 0 for k in KEYS}
last_write_time = {}
stale_counts = defaultdict(int)
total_reads = defaultdict(int)
intervals = []

state_lock = Lock()

# raw latency records
request_rows = []

# keep request-type-specific latency arrays for plotting later
read_latencies = []
write_latencies = []


class KVUser(HttpUser):
    wait_time = between(0.001, 0.01)

    @task
    def workload(self):
        if random.random() < WRITE_RATIO:
            self.write()
        else:
            self.read()

    def write(self):
        key = random.choice(KEYS)
        value = f"v{random.randint(1, 100000)}"

        start = time.time()
        with self.client.post(
            "/set",
            json={"key": key, "value": value},
            catch_response=True,
            name="/set",
        ) as res:
            latency_ms = (time.time() - start) * 1000.0

            with state_lock:
                write_latencies.append(latency_ms)

            if res.status_code != 201:
                res.failure(f"Write failed: {res.status_code}")
                self._record_request(
                    req_type="write",
                    key=key,
                    latency_ms=latency_ms,
                    stale=False,
                    returned_version="",
                    latest_version="",
                    interval_ms="",
                    status_code=res.status_code,
                )
                return

            try:
                data = res.json()
            except Exception:
                res.failure("Write response not valid JSON")
                self._record_request(
                    req_type="write",
                    key=key,
                    latency_ms=latency_ms,
                    stale=False,
                    returned_version="",
                    latest_version="",
                    interval_ms="",
                    status_code=res.status_code,
                )
                return

            version = data.get("version", 0)

            with state_lock:
                latest_versions[key] = max(latest_versions[key], version)
                last_write_time[key] = time.time()

            self._record_request(
                req_type="write",
                key=key,
                latency_ms=latency_ms,
                stale=False,
                returned_version=version,
                latest_version=version,
                interval_ms="",
                status_code=res.status_code,
            )

    def read(self):
        key = random.choice(KEYS)

        start = time.time()
        with self.client.get(
            f"/get?key={key}",
            catch_response=True,
            name="/get",
        ) as res:
            latency_ms = (time.time() - start) * 1000.0

            with state_lock:
                read_latencies.append(latency_ms)

            if res.status_code != 200:
                res.failure(f"Read failed: {res.status_code}")
                self._record_request(
                    req_type="read",
                    key=key,
                    latency_ms=latency_ms,
                    stale=False,
                    returned_version="",
                    latest_version="",
                    interval_ms="",
                    status_code=res.status_code,
                )
                return

            try:
                data = res.json()
            except Exception:
                res.failure("Read response not valid JSON")
                self._record_request(
                    req_type="read",
                    key=key,
                    latency_ms=latency_ms,
                    stale=False,
                    returned_version="",
                    latest_version="",
                    interval_ms="",
                    status_code=res.status_code,
                )
                return

            returned_version = data.get("version", 0)

            with state_lock:
                latest_version = latest_versions.get(key, 0)
                total_reads[key] += 1

                interval_ms = ""
                if key in last_write_time:
                    interval_ms = (time.time() - last_write_time[key]) * 1000.0
                    intervals.append(interval_ms)

                is_stale = returned_version < latest_version
                if is_stale:
                    stale_counts[key] += 1

            if is_stale:
                # ❗不要标 failure
                res.success()  # 仍然算成功请求
            else:
                res.success()

            self._record_request(
                req_type="read",
                key=key,
                latency_ms=latency_ms,
                stale=is_stale,
                returned_version=returned_version,
                latest_version=latest_version,
                interval_ms=interval_ms,
                status_code=res.status_code,
            )

    def _record_request(
        self,
        req_type,
        key,
        latency_ms,
        stale,
        returned_version,
        latest_version,
        interval_ms,
        status_code,
    ):
        with state_lock:
            request_rows.append({
                "timestamp": time.time(),
                "request_type": req_type,
                "key": key,
                "latency_ms": latency_ms,
                "stale": stale,
                "returned_version": returned_version,
                "latest_version": latest_version,
                "interval_ms": interval_ms,
                "status_code": status_code,
            })


# -----------------------------
# Write custom output at test end
# -----------------------------
@events.quitting.add_listener
def write_custom_outputs(environment, **kwargs):
    raw_csv = f"{RESULT_PREFIX}_requests.csv"
    stale_csv = f"{RESULT_PREFIX}_stale_summary.csv"
    interval_csv = f"{RESULT_PREFIX}_intervals.csv"
    latency_csv = f"{RESULT_PREFIX}_latencies.csv"

    # raw request-level records
    with open(raw_csv, "w", newline="") as f:
        writer = csv.DictWriter(
            f,
            fieldnames=[
                "timestamp",
                "request_type",
                "key",
                "latency_ms",
                "stale",
                "returned_version",
                "latest_version",
                "interval_ms",
                "status_code",
            ],
        )
        writer.writeheader()
        writer.writerows(request_rows)

    # stale summary
    with open(stale_csv, "w", newline="") as f:
        writer = csv.DictWriter(
            f,
            fieldnames=["key", "stale_reads", "total_reads", "stale_rate"],
        )
        writer.writeheader()

        total_stale = 0
        total_read_count = 0

        for k in KEYS:
            sr = stale_counts[k]
            tr = total_reads[k]
            rate = (sr / tr) if tr > 0 else 0.0
            total_stale += sr
            total_read_count += tr
            writer.writerow({
                "key": k,
                "stale_reads": sr,
                "total_reads": tr,
                "stale_rate": rate,
            })

        overall_rate = (total_stale / total_read_count) if total_read_count > 0 else 0.0
        writer.writerow({
            "key": "ALL",
            "stale_reads": total_stale,
            "total_reads": total_read_count,
            "stale_rate": overall_rate,
        })

    # interval distribution source
    with open(interval_csv, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["interval_ms"])
        for x in intervals:
            writer.writerow([x])

    # request-type latency source
    max_len = max(len(read_latencies), len(write_latencies), 1)
    with open(latency_csv, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["read_latency_ms", "write_latency_ms"])
        for i in range(max_len):
            r = read_latencies[i] if i < len(read_latencies) else ""
            w = write_latencies[i] if i < len(write_latencies) else ""
            writer.writerow([r, w])

    print("\\n=== Custom files written ===")
    print(raw_csv)
    print(stale_csv)
    print(interval_csv)
    print(latency_csv)