Done:
- Added support for nested indexes
- Added support for index referrers
- Added concurrent manifest fetch requests using a pool dramatically speeding up cleanup when a repo has a lot of referrers / indexes
- Migrated Purge Pool to use Pond adding support for backlog of requests
- DryRun Logic is now integrated into existing paths 

TODO:
- 429 backoffs
- More robust error handling and retries
- Testing

Left out of scope for this PR:
- Add cleanup filtering for specific artifact types
- Add cross repo delete optimizations for the pool
- Improved logging


