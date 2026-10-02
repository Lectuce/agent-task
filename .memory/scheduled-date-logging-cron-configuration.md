---
name: scheduled-date-logging-cron-configuration
description: Full crontab entry and output file details for the scheduled date logging cron job
type: project
---

The scheduled cron job that logs current date every 2 minutes has the following crontab configuration, which appends output to `~/current_date.log`:
```
*/2 * * * * date >> ~/current_date.log 2>&1
```
