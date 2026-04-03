#!/bin/bash

echo "🚀 Starting Leader-Follower Experiments..."

HOST="http://localhost:8080"
USERS=50
SPAWN_RATE=10
DURATION="60s"

RATIOS=("0.01" "0.1" "0.5" "0.9")

echo "🧹 Cleaning old result files..."
rm -f results_*

for r in "${RATIOS[@]}"
do
  echo ""
  echo "========================================"
  echo "🔥 Running test with WRITE_RATIO=$r"
  echo "========================================"

  WRITE_RATIO=$r RESULT_PREFIX=results_$r locust \
    -f locustfile.py \
    --headless \
    -u $USERS \
    -r $SPAWN_RATE \
    -t $DURATION \
    --host=$HOST \
    --csv=results_$r

  exit_code=$?

  if [ $exit_code -ne 0 ]; then
    echo "⚠️ Locust for ratio $r exited with code $exit_code"
    echo "⚠️ Continuing to next ratio..."
  else
    echo "✅ Finished ratio $r"
  fi
done

echo ""
echo "📊 Generating plots..."
python plot_results.py

echo ""
echo "🎉 ALL DONE!"