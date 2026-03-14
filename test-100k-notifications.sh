#!/usr/bin/env bash

set -u

API_URL="${API_URL:-http://localhost:8080}"
TOTAL_NOTIFICATIONS="${TOTAL_NOTIFICATIONS:-100000}"
BATCH_SIZE="${BATCH_SIZE:-1000}"
CONCURRENCY="${CONCURRENCY:-10}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-20}"
DRY_RUN="${DRY_RUN:-0}"

if ! [[ "$TOTAL_NOTIFICATIONS" =~ ^[0-9]+$ ]] || [ "$TOTAL_NOTIFICATIONS" -le 0 ]; then
  echo "TOTAL_NOTIFICATIONS must be a positive integer"
  exit 1
fi

if ! [[ "$BATCH_SIZE" =~ ^[0-9]+$ ]] || [ "$BATCH_SIZE" -le 0 ]; then
  echo "BATCH_SIZE must be a positive integer"
  exit 1
fi

if [ "$BATCH_SIZE" -gt 1000 ]; then
  echo "BATCH_SIZE cannot be greater than 1000 (API max batch size)"
  exit 1
fi

if ! [[ "$CONCURRENCY" =~ ^[0-9]+$ ]] || [ "$CONCURRENCY" -le 0 ]; then
  echo "CONCURRENCY must be a positive integer"
  exit 1
fi

if ! [[ "$TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || [ "$TIMEOUT_SECONDS" -le 0 ]; then
  echo "TIMEOUT_SECONDS must be a positive integer"
  exit 1
fi

TOTAL_BATCHES=$(( (TOTAL_NOTIFICATIONS + BATCH_SIZE - 1) / BATCH_SIZE ))
TMP_DIR="$(mktemp -d)"
START_TS="$(date +%s)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

build_payload() {
  local batch_no="$1"
  local count="$2"
  local start_idx=$(( (batch_no - 1) * BATCH_SIZE + 1 ))
  local end_idx=$(( start_idx + count - 1 ))
  local i priority recipient content

  printf '{"notifications":['
  for ((i=start_idx; i<=end_idx; i++)); do
    case $((i % 3)) in
      0) priority="high" ;;
      1) priority="medium" ;;
      2) priority="low" ;;
    esac

    recipient="loadtest+${i}@example.com"
    content="load test notification ${i}"

    printf '{"recipient":"%s","channel":"email","content":"%s","priority":"%s"}' "$recipient" "$content" "$priority"
    if [ "$i" -lt "$end_idx" ]; then
      printf ','
    fi
  done
  printf ']}'
}

send_batch() {
  local batch_no="$1"
  local count="$2"
  local payload http_code response_file batch_id
  local curl_exit=0

  response_file="$TMP_DIR/response_${batch_no}.json"
  payload="$(build_payload "$batch_no" "$count")"

  if [ "$DRY_RUN" = "1" ]; then
    http_code="201"
    printf '{"batchId":%s}' "$batch_no" > "$response_file"
  else
    http_code="$(curl -sS -m "$TIMEOUT_SECONDS" -o "$response_file" -w "%{http_code}" \
      -X POST "${API_URL}/notifications/batch" \
      -H "Content-Type: application/json" \
      -d "$payload" 2>/dev/null)" || curl_exit=$?

    if [ "$curl_exit" -ne 0 ]; then
      http_code="000"
    fi
  fi

  batch_id="$(grep -o '"batchId":[0-9]*' "$response_file" | head -1 | cut -d: -f2 || true)"

  if [ "$http_code" = "201" ]; then
    printf '%s,%s,%s,%s\n' "$batch_no" "$count" "$http_code" "${batch_id:-n/a}" > "$TMP_DIR/result_${batch_no}.txt"
  else
    printf '%s,%s,%s,%s\n' "$batch_no" "$count" "$http_code" "error" > "$TMP_DIR/result_${batch_no}.txt"
  fi
}

export API_URL BATCH_SIZE TIMEOUT_SECONDS TMP_DIR DRY_RUN
export -f build_payload send_batch

echo "Starting load script"
echo "API_URL: $API_URL"
echo "TOTAL_NOTIFICATIONS: $TOTAL_NOTIFICATIONS"
echo "BATCH_SIZE: $BATCH_SIZE"
echo "TOTAL_BATCHES: $TOTAL_BATCHES"
echo "CONCURRENCY: $CONCURRENCY"
echo "DRY_RUN: $DRY_RUN"
echo ""

for ((batch=1; batch<=TOTAL_BATCHES; batch++)); do
  remaining=$((TOTAL_NOTIFICATIONS - (batch - 1) * BATCH_SIZE))
  if [ "$remaining" -ge "$BATCH_SIZE" ]; then
    count="$BATCH_SIZE"
  else
    count="$remaining"
  fi
  printf '%s,%s\n' "$batch" "$count"
done | xargs -P "$CONCURRENCY" -n 1 -I {} bash -c 'batch_no="${1%,*}"; count="${1#*,}"; send_batch "$batch_no" "$count"' _ {}

success_batches=0
failed_batches=0
sent_notifications=0

for ((batch=1; batch<=TOTAL_BATCHES; batch++)); do
  if [ ! -f "$TMP_DIR/result_${batch}.txt" ]; then
    failed_batches=$((failed_batches + 1))
    continue
  fi

  IFS=',' read -r _batch_no count code batch_ref < "$TMP_DIR/result_${batch}.txt"

  if [ "$code" = "201" ]; then
    success_batches=$((success_batches + 1))
    sent_notifications=$((sent_notifications + count))
  else
    failed_batches=$((failed_batches + 1))
  fi
done

END_TS="$(date +%s)"
DURATION="$((END_TS - START_TS))"

echo ""
echo "Run completed"
echo "Duration: ${DURATION}s"
echo "Successful batches: $success_batches/$TOTAL_BATCHES"
echo "Failed batches: $failed_batches/$TOTAL_BATCHES"
echo "Notifications accepted (201): $sent_notifications/$TOTAL_NOTIFICATIONS"

if [ "$failed_batches" -gt 0 ]; then
  echo ""
  echo "Failed batch details (batch,count,http_code):"
  for ((batch=1; batch<=TOTAL_BATCHES; batch++)); do
    if [ -f "$TMP_DIR/result_${batch}.txt" ]; then
      IFS=',' read -r b c code _ref < "$TMP_DIR/result_${batch}.txt"
      if [ "$code" != "201" ]; then
        echo "$b,$c,$code"
      fi
    else
      remaining=$((TOTAL_NOTIFICATIONS - (batch - 1) * BATCH_SIZE))
      if [ "$remaining" -ge "$BATCH_SIZE" ]; then
        c="$BATCH_SIZE"
      else
        c="$remaining"
      fi
      echo "$batch,$c,missing-result"
    fi
  done
  exit 1
fi

