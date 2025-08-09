#!/bin/bash
set -e
set -x
while grep -q '\* \[ \]' FEATURE_CHECKLIST.md; do
    time cursor-agent -p --output-format text -f -m gpt-5 \
        "Follow these rules .cursor/rules/go-implement.mdc .cursor/rules/go-dod.mdc .cursor/rules/go-diverse-tests.mdc .cursor/rules/go-work.mdc"
    sleep 5
done

