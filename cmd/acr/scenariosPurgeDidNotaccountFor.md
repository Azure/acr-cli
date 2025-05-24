- Nested indexes were not really accounted for.
- Referrer indexes should have their children ignored too
- For one scenario, specifically where 

Purge does not carry out concurrent GET manifest operations

The concurrency flag seems to set a per purge limit so if a customer is cleaning up multiple repos at once they will run into issues with exceedingly high concurrency