Show the recent insight history to understand what's been communicated to the driver.

Run: `./bin/insightlog -limit 20`

Present the results grouped by time, showing:
- Timestamp
- Source (rule-engine, analyst, claude-analyst, driver-query)
- Priority level
- The insight message

Flag any gaps in coverage (e.g., tire wear not mentioned in last 10 minutes despite high wear).
