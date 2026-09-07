#!/bin/bash
set -e

# Reference: https://github.com/vllm-project/guidellm/tree/main/docs

# ==========================================
# 1. Default Configuration
# ==========================================
declare -a TARGET_IPS
declare -a TARGET_IDS      # optional node names paired with TARGET_IPS (result labels)
declare -a LABELS          # key=value pairs recorded in the results
CONCURRENCY_SWEEP=""       # START:END:STEP, runs the concurrent profile once per step
PORT="8000"
PROFILE="sweep"
MAX_SECONDS=30
MAX_REQUESTS=""
RATE=""
THROUGHPUT_CONCURRENCY=32   # throughput profile's max_concurrency when --rate is not given
RAMPUP=""
MODEL=""
RANDOM_SEED=""
OUTPUTS=""
INPUT_LEN=256
OUTPUT_LEN=128

# Dataset Options
DATA=""
DATA_ARGS=""
DATA_COLUMN_MAPPER=""
DATA_SAMPLES=-1
PROCESSOR=""

# ==========================================
# 2. Argument Parsing
# ==========================================
usage() {
  echo "Usage: $0 --ip <IP1> [IP2 IP3 ...] [OPTIONS]"
  echo ""
  echo "For more information, visit:"
  echo "https://github.com/vllm-project/guidellm/blob/main/docs/getting-started/benchmark.md"
  echo ""
  echo "Required:"
  echo "  --ip <IP1> [IP2 ...]   Target GPU VM IP address(es) (space-separated)"
  echo ""
  echo "Options:"
  echo "  --ids <ID1> [ID2 ...]          Node names paired with --ip, used to label results (default: IP)"
  echo "  --label <key=value>            Extra label recorded in the results (repeatable)"
  echo "  --concurrency-sweep <S:E:STEP> Run the concurrent profile at S, S+STEP, ..., E (e.g. 10:150:10),"
  echo "                                 --max-seconds applies to each step"
  echo "  --port <PORT>                  Server port. Default: $PORT"
  echo "  --profile <TYPE>               Benchmark profile (synchronous, constant, async, sweep, poisson, concurrent, throughput). Default: $PROFILE"
  echo "  --rate <RATE>                  Per-profile load level: sweep=sweep_size, constant/poisson/async=req/s,"
  echo "                                 concurrent=streams, throughput=max_concurrency (default $THROUGHPUT_CONCURRENCY);"
  echo "                                 a comma list runs one strategy per value. Required for constant/poisson/async/concurrent."
  echo "  --max-seconds <N>              Maximum duration per target in seconds. Default: $MAX_SECONDS"
  echo "  --max-requests <N>             Maximum number of requests per benchmark"
  echo "  --model <NAME>                 Model name to benchmark (e.g. Qwen/Qwen2.5-1.5B-Instruct)"
  echo "  --rampup <N>                   Ramp-up duration in seconds"
  echo "  --random-seed <N>              Random seed for reproducibility"
  echo "  --outputs <FORMATS>            Comma-separated output formats (e.g. csv,json,html). Default: csv,json"
  echo ""
  echo "Dataset Options (uses synthetic data if omitted):"
  echo "  --data <SOURCE>                Dataset source (HF dataset ID or file path)"
  echo "  --data-args <JSON>             HuggingFace load_dataset kwargs (e.g. {\"name\":\"3.0.0\",\"split\":\"test\"});"
  echo "                                 without a split, the smallest of test/validation/train is used"
  echo "  --data-column-mapper <JSON>    Dataset column mappings (e.g. {\"text_column\":\"article\"})"
  echo "  --data-samples <N>             Number of samples (-1 for all). Default: $DATA_SAMPLES"
  echo "  --processor <NAME>             Tokenizer or processor name"
  echo ""
  echo "Synthetic Data Options (used when --data is not specified):"
  echo "  --in-len <N>                   Number of input tokens. Default: $INPUT_LEN"
  echo "  --out-len <N>                  Number of output tokens. Default: $OUTPUT_LEN"
  echo ""
  echo "  -h, --help                     Show this help message"
  echo ""
  echo "Examples:"
  echo "  # Single target (synthetic data)"
  echo "  $1 --ip 1.1.1.1"
  echo ""
  echo "  # Multiple targets"
  echo "  $1 --ip 1.1.1.1 2.2.2.2 --max-seconds 120"
  echo ""
  echo "  # Concurrency sweep 10..150 step 10, 60s per step, results labeled by node name"
  echo "  $1 --ip 1.1.1.1 2.2.2.2 --ids gpu-a gpu-b --concurrency-sweep 10:150:10 --max-seconds 60"
  echo ""
  echo "  # HuggingFace dataset"
  echo "  $1 --ip 1.1.1.1 \\"
  echo "    --data 'abisee/cnn_dailymail' \\"
  echo "    --data-args '{\"name\":\"3.0.0\"}' \\"
  echo "    --data-column-mapper '{\"text_column\":\"article\"}'"
  exit 1
}

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --ip)
            shift
            # Collect all IPs until next option or end of args
            while [[ "$#" -gt 0 ]] && [[ "$1" != --* ]]; do
                TARGET_IPS+=("$1")
                shift
            done
            continue  # skip the trailing shift
            ;;
        --ids)
            shift
            while [[ "$#" -gt 0 ]] && [[ "$1" != --* ]]; do
                TARGET_IDS+=("$1")
                shift
            done
            continue
            ;;
        --label) LABELS+=("$2"); shift ;;
        --concurrency-sweep) CONCURRENCY_SWEEP="$2"; shift ;;
        --port) PORT="$2"; shift ;;
        --profile) PROFILE="$2"; shift ;;
        --max-seconds) MAX_SECONDS="$2"; shift ;;
        --max-requests) MAX_REQUESTS="$2"; shift ;;
        --rate) RATE="$2"; shift ;;
        --rampup) RAMPUP="$2"; shift ;;
        --model) MODEL="$2"; shift ;;
        --random-seed) RANDOM_SEED="$2"; shift ;;
        --outputs) OUTPUTS="$2"; shift ;;
        --in-len) INPUT_LEN="$2"; shift ;;
        --out-len) OUTPUT_LEN="$2"; shift ;;
        --data) DATA="$2"; shift ;;
        --data-args) DATA_ARGS="$2"; shift ;;
        --data-column-mapper) DATA_COLUMN_MAPPER="$2"; shift ;;
        --data-samples) DATA_SAMPLES="$2"; shift ;;
        --processor) PROCESSOR="$2"; shift ;;
        -h|--help) usage ;;
        *) echo "Error: Unknown parameter: $1"; usage ;;
    esac
    shift
done

if [ ${#TARGET_IPS[@]} -eq 0 ]; then
  echo "Error: At least one target IP address (--ip) is required."
  usage
fi
if [ ${#TARGET_IDS[@]} -gt 0 ] && [ ${#TARGET_IDS[@]} -ne ${#TARGET_IPS[@]} ]; then
  echo "Error: --ids must list one name per --ip (${#TARGET_IDS[@]} ids, ${#TARGET_IPS[@]} ips)."
  exit 1
fi

if [ -n "$CONCURRENCY_SWEEP" ]; then
  if ! [[ "$CONCURRENCY_SWEEP" =~ ^([0-9]+):([0-9]+):([0-9]+)$ ]] || [ "${BASH_REMATCH[3]}" -eq 0 ]; then
    echo "Error: --concurrency-sweep must be START:END:STEP (e.g. 10:150:10)."
    exit 1
  fi
  PROFILE="concurrent"
  RATE=$(seq -s, "${BASH_REMATCH[1]}" "${BASH_REMATCH[3]}" "${BASH_REMATCH[2]}")
fi

case "$PROFILE" in
  constant|poisson|async|concurrent)
    if [ -z "$RATE" ]; then
      echo "Error: --rate is required for the '$PROFILE' profile."
      usage
    fi ;;
esac

# ==========================================
# 3. System Requirements & Setup
# ==========================================
export DEBIAN_FRONTEND=noninteractive

install_python_venv() {
  echo "Installing Python3 and venv..."
  sudo apt-get update -qq
  PY_VER=$(python3 -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")' 2>/dev/null || echo "3")
  # venv ships pip via ensurepip; python3-pip would drag in build-essential and dev headers
  sudo apt-get install -y -qq python3 python3-venv "python${PY_VER}-venv" > /dev/null
}

if ! python3 -c "import ensurepip" >/dev/null 2>&1; then
  install_python_venv
fi

WORK_DIR="$HOME/guidellm_bench"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"

# Ensure valid venv exists (remove incomplete venv if activate is missing)
if [ -d "venv" ] && [ ! -f "venv/bin/activate" ]; then
  echo "Removing incomplete virtual environment..."
  rm -rf venv
fi

if [ ! -d "venv" ]; then
  echo "Creating virtual environment..."
  if ! python3 -m venv venv; then
    install_python_venv
    python3 -m venv venv || { echo "Error: Failed to create virtual environment"; exit 1; }
  fi
fi

source venv/bin/activate

# GuideLLM 0.7 replaced `guidellm benchmark` with `guidellm run` and a kind=...,key=value
# option format; upgrade older installs so the command below matches the installed CLI.
if guidellm run --help >/dev/null 2>&1; then
  echo "GuideLLM already installed ✓"
else
  echo "Installing GuideLLM..."
  pip install -q --upgrade pip
  pip install -q --upgrade "guidellm[recommended]>=0.7"
fi

# ==========================================
# 4. Run Benchmark
# ==========================================

# Shared run timestamp (all VMs in the same run share this)
RUN_TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Flatten benchmarks.json into CSV: one row per strategy, labels as columns,
# TTFT/TPOT/ITL/latency/throughput as mean, median, p90, p95, p99 of successful requests.
write_summary_csv() {
  python3 - "$1" <<'PY'
import csv, json, sys
j = json.load(open(sys.argv[1]))
labels = j["config"]["metadata"].get("labels") or {}
stats = ("mean", "median", "p90", "p95", "p99")
metrics = {"ttft_ms": "time_to_first_token_ms", "tpot_ms": "time_per_output_token_ms",
           "itl_ms": "inter_token_latency_ms", "request_latency_s": "request_latency",
           "output_tok_per_s": "output_tokens_per_second", "req_per_s": "requests_per_second"}
w = None
for b in j["benchmarks"]:
    st, m, rt = b["config"]["strategy"], b["metrics"], b["metrics"]["request_totals"]
    row = {**labels, "model": b["config"]["backend"].get("model", ""), "profile": st["type_"],
           "concurrency": st.get("max_concurrency") or st.get("streams") or st.get("rate"),
           "duration_s": round(b["duration"], 1),
           "req_successful": rt["successful"], "req_incomplete": rt["incomplete"], "req_errored": rt["errored"],
           "prompt_tokens_mean": round(m["prompt_token_count"]["successful"]["mean"], 1),
           "output_tokens_mean": round(m["output_token_count"]["successful"]["mean"], 1)}
    for col, key in metrics.items():
        d = m[key]["successful"]
        for s in stats:
            row[f"{col}_{s}"] = round(d["percentiles"][s] if s.startswith("p") else d[s], 3)
    if w is None:
        w = csv.DictWriter(sys.stdout, fieldnames=list(row)); w.writeheader()
    w.writerow(row)
PY
}

# Function to run benchmark for a single target IP
run_benchmark() {
  local TARGET_IP="$1"
  local TARGET_NAME="${2:-$1}"
  local TARGET_URL="http://${TARGET_IP}:${PORT}"
  # Create a unique directory for this specific run
  local RESULT_DIR="$WORK_DIR/bench_${RUN_TIMESTAMP}_${TARGET_NAME}"
  mkdir -p "$RESULT_DIR"

  # Build the data source argument dynamically
  local DATA_SOURCE
  if [ -n "$DATA" ]; then
    if [[ "$DATA" == kind=* || "$DATA" == \{* ]]; then
      DATA_SOURCE="$DATA"
    elif [ -e "$DATA" ]; then
      local kind
      case "${DATA##*.}" in
        json|jsonl) kind="json_file" ;;
        csv)        kind="csv_file" ;;
        parquet)    kind="parquet_file" ;;
        arrow)      kind="arrow_file" ;;
        *)          kind="text_file" ;;
      esac
      DATA_SOURCE="kind=${kind},path=${DATA}"
    else
      # GuideLLM 0.7 needs a single split; loading a multi-split HF dataset without one
      # fails on a DatasetDict. Pick the smallest useful split (test > validation > train)
      # from the datasets-server unless --data-args already names one.
      DATA_SOURCE=$(python3 - "$DATA" "${DATA_ARGS:-{\}}" <<'PY'
import json, sys, urllib.request
source, kwargs = sys.argv[1], json.loads(sys.argv[2])
if "split" not in kwargs:
    try:
        with urllib.request.urlopen(f"https://datasets-server.huggingface.co/splits?dataset={source}", timeout=20) as r:
            splits = [s for s in json.load(r).get("splits", [])
                      if "name" not in kwargs or s["config"] == kwargs["name"]]
        names = [s["split"] for s in splits]
        if names and len(set(names)) > 1:
            for pref in ("test", "validation", "train"):
                pick = next((n for n in names if n.startswith(pref)), None)
                if pick: break
            kwargs["split"] = pick or names[0]
    except Exception:
        pass
print(json.dumps({"kind": "huggingface", "source": source, **({"load_kwargs": kwargs} if kwargs else {})}))
PY
)
    fi
    echo "------------------------------------------"
    echo "Target:   $TARGET_NAME ($TARGET_URL)"
    echo "Profile:  $PROFILE (Max $MAX_SECONDS seconds)"
    echo "Data:     $DATA_SOURCE"
    if [ -n "$DATA_COLUMN_MAPPER" ]; then echo "  Mapper: $DATA_COLUMN_MAPPER"; fi
    if [ "$DATA_SAMPLES" != "-1" ]; then echo "  Samples: $DATA_SAMPLES"; fi
    if [ -n "$PROCESSOR" ]; then echo "  Processor: $PROCESSOR"; fi
    echo "Output:   $RESULT_DIR/"
    echo "------------------------------------------"
  else
    # If --data is not provided, construct it from synthetic data options
    DATA_SOURCE="kind=synthetic_text,prompt_tokens=${INPUT_LEN},output_tokens=${OUTPUT_LEN}"
    echo "------------------------------------------"
    echo "Target:  $TARGET_NAME ($TARGET_URL)"
    echo "Profile: $PROFILE (Max $MAX_SECONDS seconds)"
    echo "Data:    $INPUT_LEN prompt tokens / $OUTPUT_LEN output tokens (synthetic)"
    echo "Output:  $RESULT_DIR/"
    echo "------------------------------------------"
  fi

  # --rate means a different profile parameter per profile kind; a comma list becomes
  # one sub-benchmark per value via --override.
  local PROFILE_SPEC="kind=${PROFILE}" RATE_KEY="" OVERRIDE_KEY=""
  case "$PROFILE" in
    sweep)                  RATE_KEY="sweep_size" ;;
    constant|poisson|async) RATE_KEY="rate";    OVERRIDE_KEY="profile.rate" ;;
    concurrent)             RATE_KEY="streams"; OVERRIDE_KEY="profile.streams" ;;
    throughput)             RATE_KEY="max_concurrency" ;;
  esac
  local -a OVERRIDE_ARGS=()
  if [ -n "$RATE" ] && [ -n "$RATE_KEY" ]; then
    PROFILE_SPEC+=",${RATE_KEY}=${RATE%%,*}"
    if [[ "$RATE" == *,* ]] && [ -n "$OVERRIDE_KEY" ]; then
      OVERRIDE_ARGS=(--override "$OVERRIDE_KEY" "$RATE")
    fi
  elif [ "$PROFILE" = "throughput" ]; then
    PROFILE_SPEC+=",max_concurrency=${THROUGHPUT_CONCURRENCY}"
  fi
  [ -n "$RAMPUP" ] && PROFILE_SPEC+=",rampup_duration=${RAMPUP}"

  local BACKEND_SPEC="kind=openai_http,target=${TARGET_URL}"
  [ -n "$MODEL" ] && BACKEND_SPEC+=",model=${MODEL}"

  local -a GUIDELLM_CMD_ARGS=(
    guidellm run
    --backend "$BACKEND_SPEC"
    --profile "$PROFILE_SPEC"
    --data "$DATA_SOURCE"
    --disable-progress
  )
  [ ${#OVERRIDE_ARGS[@]} -gt 0 ] && GUIDELLM_CMD_ARGS+=("${OVERRIDE_ARGS[@]}")
  local lbl
  for lbl in "node=${TARGET_NAME}" "ip=${TARGET_IP}" "${LABELS[@]}"; do
    GUIDELLM_CMD_ARGS+=(--label "$lbl")
  done
  [ -n "$MAX_SECONDS" ]  && GUIDELLM_CMD_ARGS+=(--constraint "kind=max_duration,seconds=${MAX_SECONDS}")
  [ -n "$MAX_REQUESTS" ] && GUIDELLM_CMD_ARGS+=(--constraint "kind=max_requests,count=${MAX_REQUESTS}")
  [ -n "$RANDOM_SEED" ]  && GUIDELLM_CMD_ARGS+=(--seed "kind=static,value=${RANDOM_SEED}")
  [ -n "$PROCESSOR" ]    && GUIDELLM_CMD_ARGS+=(--tokenizer "kind=huggingface_auto,model=${PROCESSOR}")
  [ "$DATA_SAMPLES" != "-1" ] && GUIDELLM_CMD_ARGS+=(--data-loader "kind=pytorch,samples=${DATA_SAMPLES}")
  if [ -n "$DATA_COLUMN_MAPPER" ]; then
    # Accept the plain {"text_column":"prompt"} form and wrap it into the mapper config
    local MAPPER_SPEC="$DATA_COLUMN_MAPPER"
    if [[ "$MAPPER_SPEC" == \{* ]] && [[ "$MAPPER_SPEC" != *'"kind"'* ]]; then
      MAPPER_SPEC=$(python3 -c 'import json,sys; print(json.dumps({"kind":"generative_column_mapper","column_mappings":json.loads(sys.argv[1])}))' "$MAPPER_SPEC")
    fi
    GUIDELLM_CMD_ARGS+=(--data-column-mapper "$MAPPER_SPEC")
  fi
  local fmt
  for fmt in $(echo "${OUTPUTS:-csv,json}" | tr ',' ' '); do
    GUIDELLM_CMD_ARGS+=(--output "kind=${fmt},path=${RESULT_DIR}/benchmarks.${fmt}")
  done

  # Run the command and capture its exit code
  if ! "${GUIDELLM_CMD_ARGS[@]}"; then
    echo "Error: guidellm benchmark command failed." >&2
    # Clean up the directory if the benchmark failed
    rm -rf "$RESULT_DIR"
    return 1 # Explicitly return a failure code
  fi

  # One row per strategy with the latency metrics needed for concurrency plots
  if [ -f "$RESULT_DIR/benchmarks.json" ]; then
    write_summary_csv "$RESULT_DIR/benchmarks.json" > "$RESULT_DIR/summary.csv" \
      || echo "Warning: could not build summary.csv" >&2
  fi

  # Report only files that were actually generated
  local GENERATED_FILES=()
  for ext in json csv html; do
    if [ -f "$RESULT_DIR/benchmarks.$ext" ]; then
      GENERATED_FILES+=("$(basename "$RESULT_DIR")/benchmarks.$ext")
    fi
  done

  echo "------------------------------------------"
  echo "Benchmark completed for $TARGET_IP"
  if [ ${#GENERATED_FILES[@]} -gt 0 ]; then
    for f in "${GENERATED_FILES[@]}"; do echo "  $f"; done
  else
    echo "  (no output files generated)"
  fi
  for ext in json csv html; do
    if [ -f "$RESULT_DIR/benchmarks.$ext" ]; then
      echo "\$\$FILEPATH[Results ${ext^^}]($RESULT_DIR/benchmarks.$ext)"
    fi
  done
  [ -f "$RESULT_DIR/summary.csv" ] && echo "\$\$FILEPATH[Summary CSV]($RESULT_DIR/summary.csv)"
  echo "------------------------------------------"
}

# Run benchmarks for all target IPs
echo "=========================================="
echo "GuideLLM Benchmark"
echo "Targets: ${#TARGET_IPS[@]} node(s)"
for i in "${!TARGET_IPS[@]}"; do echo "  - ${TARGET_IDS[$i]:-${TARGET_IPS[$i]}} (${TARGET_IPS[$i]})"; done
[ -n "$CONCURRENCY_SWEEP" ] && echo "Concurrency sweep: $RATE (${MAX_SECONDS}s each)"
echo "=========================================="

TOTAL=${#TARGET_IPS[@]}
FAILED=0

declare -A PIDS          # PID -> IP mapping
declare -A LOG_FILES     # IP -> log file mapping

echo ""
for i in "${!TARGET_IPS[@]}"; do
  ip="${TARGET_IPS[$i]}"
  name="${TARGET_IDS[$i]:-$ip}"
  LOG_FILE="$WORK_DIR/.bench_log_${RUN_TIMESTAMP}_${name}.log"
  LOG_FILES["$name"]="$LOG_FILE"

  echo "  Starting benchmark for $name ($ip) ..."
  run_benchmark "$ip" "$name" > "$LOG_FILE" 2>&1 &
  PIDS[$!]="$name"
done

echo ""
echo "Waiting for ${#PIDS[@]} benchmark(s) to complete..."
echo ""

# Wait for all background jobs and collect results
for pid in "${!PIDS[@]}"; do
  ip="${PIDS[$pid]}"
  
  if wait "$pid"; then
    STATUS_MSG="COMPLETED"
  else
    STATUS_MSG="FAILED"
    FAILED=$((FAILED + 1))
  fi

  echo ""
  echo "[$STATUS_MSG] $ip"
  if [ -f "${LOG_FILES[$ip]}" ]; then
    sed "s/^/[ $ip ] /" "${LOG_FILES[$ip]}"
    rm -f "${LOG_FILES[$ip]}"
  else
    echo "[ $ip ] (No log output found)"
  fi
done

# ==========================================
# 5. Summary
# ==========================================

# Collect all result directories from this run
RESULT_DIRS=($(ls -1d "$WORK_DIR"/bench_${RUN_TIMESTAMP}_* 2>/dev/null || true))

echo ""
echo "Summary: $TOTAL target(s), $((TOTAL - FAILED)) succeeded, $FAILED failed"

if [ ${#RESULT_DIRS[@]} -gt 0 ]; then
  for d in "${RESULT_DIRS[@]}"; do
    echo "  $(basename "$d")"
  done

  # Merge per-target summaries so one CSV holds every node and concurrency step
  SUMMARY_FILE="$WORK_DIR/bench_${RUN_TIMESTAMP}_summary.csv"
  awk 'FNR==1 && NR!=1 {next} {print}' "$WORK_DIR"/bench_${RUN_TIMESTAMP}_*/summary.csv > "$SUMMARY_FILE" 2>/dev/null \
    && echo "\$\$FILEPATH[Summary CSV (all targets)]($SUMMARY_FILE)"

  # Compress all result directories into a single zip for bulk download
  ZIP_NAME="bench_${RUN_TIMESTAMP}_all.zip"
  ZIP_FILE="$WORK_DIR/$ZIP_NAME"
  if ! command -v zip >/dev/null 2>&1; then
    echo "  Installing zip..."
    sudo apt-get update -qq
    sudo apt-get install -y zip -qq
  fi
  (cd "$WORK_DIR" && zip -r "$ZIP_NAME" bench_${RUN_TIMESTAMP}_*/ > /dev/null)
  ZIP_SIZE=$(du -sh "$ZIP_FILE" | cut -f1)
  echo "  Archive: $ZIP_NAME ($ZIP_SIZE)"
  echo "\$\$FILEPATH[Download All Results (zip)]($ZIP_FILE)"
else
  echo "  No result files generated."
fi

[ $FAILED -eq 0 ] || exit 1
