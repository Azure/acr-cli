- Nested indexes were not really accounted for.
- Referrer indexes should have their children ignored too
- For one scenario, specifically where 

Purge does not carry out concurrent GET manifest operations

The concurrency flag seems to set a per purge limit so if a customer is cleaning up multiple repos at once they will run into issues with exceedingly high concurrency

TODO:
- Nested Indexes - Done
- Referrer Indexes - Done
- Pool for Gets - Done
- Pool for Deletes - Rewrote, but no cross repo scenarios

- Certain artifact types - Next PR

Optimize the cross repo scenario?
We shouldn't do one repo one by one, instead we can spread the messages across repos for deletes. This will allow the greatest throughtput
- Slowdowns for 429s?

Testing


