from locust import HttpUser, task, between, events
import random
import os
import csv
import time
from collections import defaultdict
from threading import Lock

# -----------------------------
# Leaderless test config
# -----------------------------
NODES = [
    "http://localhost:8080",
    "http://localhost:8081",
    "http://localhost:8082",
    "http://localhost:8083",
    "http://localhost:8084",
]

# small hot set to ensure local-in-time behavior
KEYS = ["k1", "k2", "k3", "k4", "k5"]

WRITE_RATIO = float(os.getenv("WRITE_RATIO", "0.1"))
RESULT_PREFIX = os.getenv("RESULT_PREFIX", f"leaderless_{WRITE_RATIO}")

# latest version seen globally by the client
latest_versions = {k: 0 for k in KEYS}
last_write_time = {}
stale_counts = defaultdict(int)
total_reads = defaultdict(int)
intervals = []

request_rows = []
read_latencies = []
write_latencies = []

state_lock = Lock()


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
        node = random.choice(NODES)  # any node can become coordinator

        start = time.time()
        with self.client.post(
            node + "/set",
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
                    node=node,
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
                    node=node,
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
                node=node,
                key=key,
                latency_ms=latency_ms,
                stale=False,
                returned_version=version,
                latest_version=version,
                interval_ms="",
                status_code=res.status_code,
            )
            res.success()

    def read(self):
        key = random.choice(KEYS)
        node = random.choice(NODES)  # any node can serve a read

        start = time.time()
        with self.client.get(
            node + f"/get?key={key}",
            catch_response=True,
            name="/get",
        ) as res:
            latency_ms = (time.time() - start) * 1000.0

            with state_lock:
                read_latencies.append(latency_ms)

            if res.status_code != 200:
                # key may temporarily be missing early in a run; treat as failure for observability
                res.failure(f"Read failed: {res.status_code}")
                self._record_request(
                    req_type="read",
                    node=node,
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
                    node=node,
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

            # stale read is an experimental signal, not an HTTP failure
            res.success()

            self._record_request(
                req_type="read",
                node=node,
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
        node,
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
                "node": node,
                "key": key,
                "latency_ms": latency_ms,
                "stale": stale,
                "returned_version": returned_version,
                "latest_version": latest_version,
                "interval_ms": interval_ms,
                "status_code": status_code,
            })


@events.quitting.add_listener
def write_custom_outputs(environment, **kwargs):
    raw_csv = f"{RESULT_PREFIX}_requests.csv"
    stale_csv = f"{RESULT_PREFIX}_stale_summary.csv"
    interval_csv = f"{RESULT_PREFIX}_intervals.csv"
    latency_csv = f"{RESULT_PREFIX}_latencies.csv"

    with open(raw_csv, "w", newline="") as f:
        writer = csv.DictWriter(
            f,
            fieldnames=[
                "timestamp",
                "request_type",
                "node",
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

    with open(interval_csv, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["interval_ms"])
        for x in intervals:
            writer.writerow([x])

    max_len = max(len(read_latencies), len(write_latencies), 1)
    with open(latency_csv, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["read_latency_ms", "write_latency_ms"])
        for i in range(max_len):
            r = read_latencies[i] if i < len(read_latencies) else ""
            w = write_latencies[i] if i < len(write_latencies) else ""
            writer.writerow([r, w])

    print("\n=== Custom files written ===")
    print(raw_csv)
    print(stale_csv)
    print(interval_csv)
    print(latency_csv)
