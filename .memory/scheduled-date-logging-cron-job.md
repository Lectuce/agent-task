---
name: scheduled-date-logging-cron-job
description: Fact of a successfully scheduled cron job that logs current date every 2 minutes
type: project
---

A cron job has been successfully completed and scheduled to print the current date every 2 minutes. The job appends all output to `~/current_date.log` for later checking. The crontab entry for this job is:
```
*/2 * * * * date >> ~/current_date.log 2>&1
```
