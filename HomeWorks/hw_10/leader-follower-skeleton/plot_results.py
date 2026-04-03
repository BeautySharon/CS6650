import pandas as pd
import matplotlib.pyplot as plt
import os

RATIOS = ["0.01", "0.1", "0.5", "0.9"]


def safe_read_csv(path):
    if not os.path.exists(path):
        print(f"Missing file: {path}")
        return None
    return pd.read_csv(path)


summary_rows = []
per_ratio_data = {}

for ratio in RATIOS:
    stats_file = f"results_{ratio}_stats.csv"
    stale_file = f"results_{ratio}_stale_summary.csv"
    interval_file = f"results_{ratio}_intervals.csv"
    request_file = f"results_{ratio}_requests.csv"

    stats_df = safe_read_csv(stats_file)
    stale_df = safe_read_csv(stale_file)
    interval_df = safe_read_csv(interval_file)
    request_df = safe_read_csv(request_file)

    if stats_df is None or stale_df is None or interval_df is None or request_df is None:
        print(f"Skipping ratio {ratio} because required files are missing.")
        continue

    read_rows = stats_df[stats_df["Name"].astype(str).str.startswith("/get")]
    write_row = stats_df[stats_df["Name"] == "/set"]

    if read_rows.empty or write_row.empty:
        print(f"Skipping summary for ratio {ratio}: missing read/write rows")
        continue

    read_avg = read_rows["Average Response Time"].mean()
    read_p95 = read_rows["95%"].mean()
    write_avg = write_row["Average Response Time"].iloc[0]
    write_p95 = write_row["95%"].iloc[0]

    stale_all = stale_df[stale_df["key"] == "ALL"]
    stale_reads = int(stale_all["stale_reads"].iloc[0]) if not stale_all.empty else 0
    total_reads = int(stale_all["total_reads"].iloc[0]) if not stale_all.empty else 0
    stale_rate = float(stale_all["stale_rate"].iloc[0]) if not stale_all.empty else 0.0

    summary_rows.append({
        "write_ratio": ratio,
        "read_avg_ms": read_avg,
        "write_avg_ms": write_avg,
        "read_p95_ms": read_p95,
        "write_p95_ms": write_p95,
        "stale_reads": stale_reads,
        "total_reads": total_reads,
        "stale_rate": stale_rate,
    })

    read_req = request_df[request_df["request_type"] == "read"].copy()
    write_req = request_df[request_df["request_type"] == "write"].copy()
    interval_values = interval_df["interval_ms"].dropna()

    per_ratio_data[ratio] = {
        "read_req": read_req,
        "write_req": write_req,
        "interval_values": interval_values,
    }

summary_df = pd.DataFrame(summary_rows)
print("\n=== Summary Table ===")
print(summary_df)

if summary_df.empty:
    print("No summary data available.")
    raise SystemExit(0)

summary_df.to_csv("summary_metrics.csv", index=False)

# -----------------------------
# Figure 1: Summary dashboard
# -----------------------------
x = range(len(summary_df))
width = 0.35

fig, axes = plt.subplots(2, 2, figsize=(14, 10))

# Average latency
axes[0, 0].bar([i - width / 2 for i in x], summary_df["read_avg_ms"], width=width, label="Read Avg")
axes[0, 0].bar([i + width / 2 for i in x], summary_df["write_avg_ms"], width=width, label="Write Avg")
axes[0, 0].set_xticks(list(x))
axes[0, 0].set_xticklabels(summary_df["write_ratio"])
axes[0, 0].set_title("Average Latency")
axes[0, 0].set_xlabel("Write Ratio")
axes[0, 0].set_ylabel("Latency (ms)")
axes[0, 0].legend()

# P95 latency
axes[0, 1].bar([i - width / 2 for i in x], summary_df["read_p95_ms"], width=width, label="Read P95")
axes[0, 1].bar([i + width / 2 for i in x], summary_df["write_p95_ms"], width=width, label="Write P95")
axes[0, 1].set_xticks(list(x))
axes[0, 1].set_xticklabels(summary_df["write_ratio"])
axes[0, 1].set_title("P95 Latency")
axes[0, 1].set_xlabel("Write Ratio")
axes[0, 1].set_ylabel("Latency (ms)")
axes[0, 1].legend()

# Stale read count
axes[1, 0].bar(summary_df["write_ratio"], summary_df["stale_reads"])
axes[1, 0].set_title("Stale Reads")
axes[1, 0].set_xlabel("Write Ratio")
axes[1, 0].set_ylabel("Count")

# Stale read rate
axes[1, 1].bar(summary_df["write_ratio"], summary_df["stale_rate"])
axes[1, 1].set_title("Stale Read Rate")
axes[1, 1].set_xlabel("Write Ratio")
axes[1, 1].set_ylabel("Rate")

plt.suptitle("W=3, R=3 Summary Dashboard", fontsize=14)
plt.tight_layout(rect=[0, 0, 1, 0.97])
plt.savefig("summary_dashboard.png")
plt.close()

# -----------------------------
# Figure 2: Distribution dashboard
# rows = ratios
# cols = read latency, write latency, interval
# -----------------------------
valid_ratios = [r for r in RATIOS if r in per_ratio_data]
fig, axes = plt.subplots(len(valid_ratios), 3, figsize=(18, 4 * len(valid_ratios)))

# handle case when only one ratio exists
if len(valid_ratios) == 1:
    axes = [axes]

for row_idx, ratio in enumerate(valid_ratios):
    row_axes = axes[row_idx]
    read_req = per_ratio_data[ratio]["read_req"]
    write_req = per_ratio_data[ratio]["write_req"]
    interval_values = per_ratio_data[ratio]["interval_values"]

    # Read latency
    if not read_req.empty:
        row_axes[0].hist(read_req["latency_ms"], bins=40)
    row_axes[0].set_title(f"Read Latency (ratio={ratio})")
    row_axes[0].set_xlabel("Latency (ms)")
    row_axes[0].set_ylabel("Frequency")

    # Write latency
    if not write_req.empty:
        row_axes[1].hist(write_req["latency_ms"], bins=40)
    row_axes[1].set_title(f"Write Latency (ratio={ratio})")
    row_axes[1].set_xlabel("Latency (ms)")
    row_axes[1].set_ylabel("Frequency")

    # Interval
    if not interval_values.empty:
        row_axes[2].hist(interval_values, bins=40)
    row_axes[2].set_title(f"Read-Write Interval (ratio={ratio})")
    row_axes[2].set_xlabel("Time since last write to same key (ms)")
    row_axes[2].set_ylabel("Frequency")

plt.suptitle("W=3, R=3 Distribution Dashboard", fontsize=16)
plt.tight_layout(rect=[0, 0, 1, 0.97])
plt.savefig("distribution_dashboard.png")
plt.close()

print("\nSaved files:")
print("- summary_metrics.csv")
print("- summary_dashboard.png")
print("- distribution_dashboard.png")