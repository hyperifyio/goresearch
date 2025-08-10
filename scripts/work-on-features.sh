#!/bin/bash
set -e
set -x
while grep -q '\* \[ \]' FEATURE_CHECKLIST.md; do

    echo
    echo "--- WORKING ON ---"
    echo

    time cursor-agent -p --output-format text -f -m gpt-5 \
        "Follow these rules .cursor/rules/go-implement.mdc .cursor/rules/go-dod.mdc .cursor/rules/go-diverse-tests.mdc .cursor/rules/go-work.mdc"

    if ! git diff --quiet || ! git diff --cached --quiet; then
        echo
        echo "--- COMMITTING UNCHANGED TO GIT ---"
        echo
        git add .
        cursor-agent -p --output-format text -f -m gpt-5 \
            "Follor these rules .cursor/rules/go-commit.mdc"
    fi

    sleep 5
done

