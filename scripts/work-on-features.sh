#!/bin/bash
set -e
set -x
while grep -q '\* \[ \]' FEATURE_CHECKLIST.md; do

    if ! git diff --quiet || ! git diff --cached --quiet; then
        echo
        echo "--- COMMITTING UNCHANGED TO GIT ---"
        echo
        git add .
        cursor-agent -p --output-format text -f -m gpt-5 \
            "Follor these rules .cursor/rules/go-commit.mdc"
    fi

    echo
    echo "--- WORKING ON ---"
    echo

    time cursor-agent -p --output-format text -f -m gpt-5 \
        "You have full access to shell commands like 'git' and 'docker'. You MUST system test everything with real services, and expect existing implementations not been finished and require fixing. Follow these rules .cursor/rules/go-implement.mdc .cursor/rules/go-dod.mdc .cursor/rules/go-diverse-tests.mdc .cursor/rules/go-work.mdc"

    sleep 5
done

