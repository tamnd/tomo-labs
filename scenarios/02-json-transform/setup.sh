#!/usr/bin/env bash
# A real-ish HR export: fifteen employees across departments, each with a
# salary. The agent has to filter one department and rank it.
set -e
W="$1"
cat > "$W/employees.json" <<'JSON'
[
  {"name": "Alice Tran", "department": "Engineering", "salary": 145000},
  {"name": "Bob Ng", "department": "Sales", "salary": 90000},
  {"name": "Carol Diaz", "department": "Engineering", "salary": 160000},
  {"name": "Dave Kim", "department": "Marketing", "salary": 85000},
  {"name": "Eve Larsson", "department": "Engineering", "salary": 132000},
  {"name": "Frank Alvarez", "department": "Sales", "salary": 98000},
  {"name": "Grace Okafor", "department": "Engineering", "salary": 175000},
  {"name": "Heidi Petrov", "department": "Support", "salary": 72000},
  {"name": "Ivan Novak", "department": "Engineering", "salary": 118000},
  {"name": "Judy Chen", "department": "Marketing", "salary": 91000},
  {"name": "Mallory Singh", "department": "Engineering", "salary": 140000},
  {"name": "Niaj Rahman", "department": "Support", "salary": 68000},
  {"name": "Olivia Rossi", "department": "Sales", "salary": 105000},
  {"name": "Peggy Adeyemi", "department": "Engineering", "salary": 152000},
  {"name": "Trent Bauer", "department": "Marketing", "salary": 88000}
]
JSON
