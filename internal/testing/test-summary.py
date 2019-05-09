# Run like:
#  go test  -json ./... | python test-summary.py

import sys
import json


passed = 0
failed = 0
skipped = 0

for line in sys.stdin:
    test = json.loads(line)
    action = test["Action"]
    if action == "pass":
        passed += 1
    elif action == "fail":
        failed += 1
    elif action == "skip":
        skipped += 1

total = passed + failed  +skipped
print("ran", total, "passed", passed, "failed", failed, "skipped", skipped)
