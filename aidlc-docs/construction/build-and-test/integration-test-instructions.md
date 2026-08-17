# Integration Test Instructions

## Purpose

Verify the full stack — repository SQL → `InsightsService` aggregation → Gin handler → embedded template — produces a correct dashboard. The unit tests stop at the service layer; the handler and template layers are only covered here.

**Status**: these are **manual procedures**, executed once for this unit. They are not automated.

## Safety: never test against the real database

The application resolves `privateledger.db` and `config.json` **beside the binary**. Running a build from the repo root therefore opens your real financial data. Every procedure below runs from a scratch directory against a copy.

### Setup

```bash
WORK=$(mktemp -d)
go build -o "$WORK/privateledger" ./cmd/privateledger
cp privateledger.db "$WORK/privateledger.db"      # a copy; the original is never touched
cat > "$WORK/config.json" <<'EOF'
{
  "version": 1,
  "server": { "port": 8899, "auto_open_browser": false },
  "logging": { "enable_file_logging": false, "log_file_path": "server.log", "log_level": "warn" },
  "start_of_month": 1,
  "debug_mode": false
}
EOF
(cd "$WORK" && ./privateledger &)
```

Port `8899` avoids colliding with a real instance on the default `8844`. `auto_open_browser: false` keeps it headless.

### Teardown

```bash
kill $(lsof -ti tcp:8899); rm -rf "$WORK"
```

---

## Scenario 1: Template renders (handler → template)

Catches malformed templates, which are **not** build errors and otherwise panic on first render.

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8899/
```

- **Expected**: `200`
- **On failure**: a `500` plus a panic in the server log means a template syntax error.

**Result when executed**: 200, 68,307 bytes.

---

## Scenario 2: Uncategorized rows reach the HTML (service → template)

```bash
curl -s http://localhost:8899/ > dash.html
grep -o "Uncategorized [A-Za-z]*" dash.html | sort | uniq -c
grep -c 'data-testid="uncategorized-expense-link"' dash.html
grep -c 'data-testid="uncategorized-income-link"' dash.html
```

- **Expected**: one `Uncategorized Expenses`, one `Uncategorized Incomes`; both `data-testid` attributes present.

**Result when executed**: both rows present; 2 expense links, 2 income links.

---

## Scenario 3: No double-counting in the API payload

The invariant that the column total equals the sum of its rows, checked against real data across all three tables.

```bash
curl -s http://localhost:8899/api/insights/dashboard > dash.json
python3 - dash.json <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
for key in ("expense_breakdown","income_breakdown","investment_breakdown"):
    t=d[key]; ok=True
    for p in t["periods"]:
        lbl=p["label"]
        s=sum(c["monthly_totals"].get(lbl,0) for c in t["categories"])
        if abs(s-t["monthly_totals"].get(lbl,0))>0.005:
            ok=False; print(f"  MISMATCH {key} {lbl}")
    print(f"{key}: {'OK' if ok else 'FAIL'}")
PY
```

**Result when executed**: OK for all three tables.

---

## Scenario 4: Figures match the database

Cross-checks the API against raw SQL. **Use the correct `category_type` values** — `1=General, 2=Expense, 3=Income, 4=Investment`. An earlier run of this check used `3` for Investment and silently passed against zero rows; the mistake is easy to make and produces a false positive.

```bash
python3 - dash.json "$WORK/privateledger.db" <<'PY'
import json,sys,sqlite3
d=json.load(open(sys.argv[1])); db=sqlite3.connect(sys.argv[2])
def q(sql,*a): return db.execute(sql,a).fetchone()[0] or 0
cur=d["current_month"]["period"]; s,e=cur["start_date"],cur["end_date"]
for key,ct,tt in (("expense_summary",2,1),("income_summary",3,2),("investment_summary",4,None)):
    cat=q("SELECT SUM(t.amount) FROM ledger_transaction t JOIN category c ON t.category_id=c.category_id "
          "WHERE c.category_type=? AND t.date_posted>=? AND t.date_posted<=?",ct,s,e)
    unc=q("SELECT SUM(amount) FROM ledger_transaction WHERE category_id IS NULL AND transaction_type=? "
          "AND date_posted>=? AND date_posted<=?",tt,s,e) if tt else 0
    exp=abs(cat+unc); got=d[key]["total_amount"]
    print(f"{key}: card={got:.2f} expected={exp:.2f} {'OK' if abs(exp-got)<0.005 else 'FAIL'}")
PY
```

**Result when executed**: all three OK. Expense `698.27` (202.97 categorized + 495.30 uncategorized); income `936.21`; investment `0.00`.

**Caveat**: the test database has no categorized income and no investment transactions, so the income and investment assertions are weak here — both sides are zero. Those paths are covered properly by the unit tests, which build the fixtures explicitly.

---

## Scenario 5: Uncategorized links filter correctly (template → handler → repository)

```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  "http://localhost:8899/transactions?uncategorized=true&start_date=2026-06-01&end_date=2026-06-30"

curl -s "http://localhost:8899/api/transactions?uncategorized=true&start_date=2026-06-01&end_date=2026-06-30" \
| python3 -c "
import json,sys
rows=json.load(sys.stdin)
rows=rows if isinstance(rows,list) else rows.get('transactions',[])
bad_cat=[r for r in rows if r.get('category_id') is not None and r.get('category_source',0)!=0]
bad_date=[r for r in rows if not ('2026-06-01'<=r['date_posted'][:10]<='2026-06-30')]
print(f'returned={len(rows)} wrong-category={len(bad_cat)} out-of-range={len(bad_date)}')"
```

- **Expected**: `200`; zero wrong-category and zero out-of-range rows.

**Result when executed**: 200; 56 transactions, 0 wrong-category, 0 out-of-range.

---

## Not covered

- **Visual appearance** — muted italic styling, and whether the dark-grey pie slice (`#4b5563`) reads well beside the existing palette. Requires a human looking at the page.
- **Bar chart rendering** — verified indirectly via `monthlyTotals` in the API payload, not by inspecting the chart.
- **Custom `start_of_month`** — all procedures ran with `start_of_month: 1`. Period-boundary behavior for pay-cycle configurations (e.g. `19`) is untested for this feature.
- **Empty database** — behavior with no transactions at all was not exercised end-to-end.
