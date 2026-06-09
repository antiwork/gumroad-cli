#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "usage: $0 <tag>" >&2
    exit 1
fi

tag=$1

if [[ ! $tag =~ ^v0\.([0-9]{4})([0-9]{2})([0-9]{2})\.([0-9]|[1-9][0-9]*)$ ]]; then
    cat >&2 <<'EOF'
Error: release tags must use Go-compatible date versioning.

Expected: v0.YYYYMMDD.N
Example:  v0.20260609.0

N starts at 0 and increments only for multiple releases on the same UTC date.
EOF
    exit 1
fi

year=${BASH_REMATCH[1]}
month=${BASH_REMATCH[2]}
day=${BASH_REMATCH[3]}

year_num=$((10#$year))
month_num=$((10#$month))
day_num=$((10#$day))

if (( month_num < 1 || month_num > 12 )); then
    echo "Error: ${year}-${month}-${day} is not a valid UTC release date." >&2
    exit 1
fi

days_in_month=(0 31 28 31 30 31 30 31 31 30 31 30 31)
if (( month_num == 2 && (year_num % 400 == 0 || (year_num % 4 == 0 && year_num % 100 != 0)) )); then
    days_in_month[2]=29
fi

if (( day_num < 1 || day_num > days_in_month[month_num] )); then
    echo "Error: ${year}-${month}-${day} is not a valid UTC release date." >&2
    exit 1
fi
