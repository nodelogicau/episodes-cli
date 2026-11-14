## ADDED Requirements

### Requirement: Path derives from id
An episode whose id is a UUIDv7 SHALL live at `episodes/<YYYY-MM>/<id>.yaml`, where the month is the UTC month of the id's minting instant, whatever its `occurred` or `timestamp`. Any other id SHALL live at `episodes/legacy/<id>.yaml`. A file elsewhere SHALL be an error in `validate`, naming the file and the path its id derives.

#### Scenario: Recorded long after it happened
- **WHEN** an email from 2025 is recorded on 2026-11-07 UTC
- **THEN** its file is under `episodes/2026-11/`

#### Scenario: Misplaced file
- **WHEN** a file minted in November is found in `episodes/2026-10/`
- **THEN** `validate` reports an error naming both paths

### Requirement: Sealing
A month SHALL be sealed once any id in the workspace was minted two or more months after it. The newest id's month and the month before it are open. `legacy` is never sealed. A writer SHALL refuse to mint into a sealed month.

#### Scenario: Grace month
- **WHEN** the newest id is from December
- **THEN** November and December are open and October is sealed

#### Scenario: Clock behind a sealed month
- **WHEN** the newest id is from 2027-01 and the process clock reads 2026-11
- **THEN** every recording verb exits 2 naming the sealed month and the clock

### Requirement: Late-merge detection by count
For each sealed month, the number of files in its directory SHALL be compared with its segment's `count`. A difference SHALL be an error in `validate` and in `index`, naming the files not present in the segment and the remedy: re-mint the branch's episodes before merging.

#### Scenario: Branch open across two boundaries
- **WHEN** a merge adds a file to a sealed month's directory
- **THEN** `validate` exits 4 naming that file

### Requirement: Broken clocks
`validate` SHALL warn on any id minted, or `timestamp` dated, later than now, and SHALL report an error for any id minted two or more months after the current month.

#### Scenario: Id from far ahead
- **WHEN** an id minted in 2027-03 is present and now is 2026-11
- **THEN** `validate` reports an error naming it
