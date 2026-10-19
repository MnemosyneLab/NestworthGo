"""Rebuild synthetic primitive-write CSV/summary from the committed raw log."""
import csv
import json
import re
from pathlib import Path

root = Path(__file__).parent
rows = []
for line in (root / 'write-amplification.log').read_text().splitlines():
    if 'write scenario=' in line:
        rows.append(dict(re.findall(r'(\w+)=([^ ]+)', line)))
assert len(rows) == 120
with (root / 'write-samples.csv').open('w') as output:
    writer = csv.DictWriter(output, fieldnames=list(rows[0]))
    writer.writeheader()
    writer.writerows(rows)
summary = []
for scenario in ('quotes-100', 'catalog-100'):
    for path in ('0', '1'):
        group = [row for row in rows if row['scenario'] == scenario and row['path'] == path]
        assert len(group) == 30
        item = dict(scenario=scenario, path=path, n=len(group))
        for field in list(group[0])[3:]:
            values = sorted(int(row[field]) for row in group)
            item[field] = dict(min=values[0], median=values[14], p95=values[28], max=values[-1])
        summary.append(item)
(root / 'write-summary.json').write_text(json.dumps(summary, indent=2) + '\n')
