- Nested indexes were not really accounted for.
- Referrer indexes should have their children ignored too
- For one scenario, specifically where 

- tags deletes requires re listing tags multiple times, once for each regex, we should do all at once

Purge does not carry out concurrent GET manifest operations