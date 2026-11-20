## MODIFIED Requirements

### Requirement: The none mode
`content: {mode: none}` SHALL be written only by `conversation start`, for the container. It means a container with nothing of its own. It SHALL NOT carry `language` or any other content field.

#### Scenario: Language on none refused
- **WHEN** a `mode: none` episode carries `language: en`
- **THEN** `validate` reports the error `language_on_none`

## ADDED Requirements

### Requirement: The attested mode
`content: {mode: attested}` marks a self-attested act and SHALL be written only by `act`. Content with `mode: attested` SHALL carry no other key, `language` included. A writer SHALL refuse one that does, and `validate` SHALL report an error naming each extra field. Readers (`show`, `list`, `thread`, `index`) SHALL accept `attested` wherever they read `content.mode`.

#### Scenario: Language on an act refused
- **WHEN** `act … --language en` is run
- **THEN** the write is refused with exit 2, naming mode attested

#### Scenario: Extra key on attested
- **WHEN** a hand-written episode has `content: {mode: attested, language: en}`
- **THEN** `validate` reports the error `attested_with_content` on `content.language`

#### Scenario: An act shown
- **WHEN** `list` is run over a workspace holding an act
- **THEN** the act is listed with `mode: attested`
