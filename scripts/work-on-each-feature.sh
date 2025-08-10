#!/bin/bash
set -e
set -x

commit_if_changes() {
  if ! git diff --quiet || ! git diff --cached --quiet; then
    echo
    echo "--- COMMITTING LOCAL CHANGES ---"
    echo
    git add .
    cursor-agent -p --output-format text -f -m gpt-5 \
      "Follow these rules .cursor/rules/go-commit.mdc"
  fi
}

while grep -Eq '^\s*\*\s\[\s\]' FEATURE_CHECKLIST.md; do
  commit_if_changes

  # Pick the first unchecked checklist item
  line=$(grep -nE '^\s*\*\s\[\s\]' FEATURE_CHECKLIST.md | head -n1)
  line_num=${line%%:*}
  raw_line=${line#*:}
  task_text=$(printf '%s\n' "$raw_line" | sed 's/^[[:space:]]*\*\s\[\s\]\s*//')

  echo
  echo "--- WORKING ON (#$line_num): $task_text ---"
  echo

  prompt=$(cat <<EOF
Follow these rules .cursor/rules/go-implement.mdc .cursor/rules/go-dod.mdc .cursor/rules/go-diverse-tests.mdc .cursor/rules/go-work.mdc

Implement the following single FEATURE_CHECKLIST item now — focus ONLY on this item:

${task_text}

When implementation and tests are complete, update FEATURE_CHECKLIST.md to check off this exact line. Keep commits small and meaningful.
EOF
)

  time cursor-agent -p --output-format text -f -m gpt-5 "$prompt"

  # Optionally commit after each attempt as well
  commit_if_changes

  sleep 5
done

echo "All checklist items completed or none remain unchecked."
