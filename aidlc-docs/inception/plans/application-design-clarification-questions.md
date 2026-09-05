# Application Design Clarification Questions — Issue #5 SIC Auto-Categorization

I detected ambiguity in the Application Design answers, especially around showing SIC in existing modals and creating SIC mappings from those modal workflows.

Please answer each question by filling in the letter choice after the `[Answer]:` tag. If none of the options match, choose `X` and describe your preference after the tag.

## Question 1
In your answer to Application Design Question 7, what should “default description” mean when showing a SIC code in the Change Category and Create Categorization Pattern modals?

A) Show the SIC code plus the current SIC mapping state: mapped category name, empty-category mapping, or unmapped

B) Show a human-readable SIC description/name from the mapping file, in addition to the SIC code and mapping state

C) Show only the SIC code; do not add a separate description field

X) Other (please describe after [Answer]: tag below)

[Answer]: X. The SIC mapping table (and also the initial CSV) will have following data columns (in addition to any required ID column): SIC_Code, Description, Descrition_Detail, Category_ID. The UI should show the Description column value. If the Description column value  is empty, it should show the Description_Detail value.

## Question 2
When the user changes a transaction category in the Change Category modal and agrees to create/update the SIC-to-category mapping, what should happen to the current transaction?

A) Apply the selected category manually to the current transaction, and also create/update the SIC mapping for future uncategorized transactions

B) Create/update the SIC mapping, then apply that SIC rule to the current transaction with rule source instead of manual source

C) Create/update the SIC mapping only; do not change the current transaction category in that action

X) Other (please describe after [Answer]: tag below)

[Answer]: B

## Question 3
When the user is in the Create Categorization Pattern modal for a transaction with a SIC code and accepts the prompt to create a SIC mapping instead, should a text pattern still be created?

A) No, create/update only the SIC mapping and do not create a text pattern

B) Yes, create both the SIC mapping and the text pattern

C) Ask the user to choose between SIC mapping only, text pattern only, or both

X) Other (please describe after [Answer]: tag below)

[Answer]: A

## Question 4
Should creating/updating a SIC mapping from either modal support empty category mappings?

A) No, modal-created SIC mappings require a selected category because the user is explicitly assigning one

B) Yes, allow the user to choose an empty category from the prompt so the SIC code intentionally does not categorize

C) Only the dedicated SIC mapping page and mapping file upload can create empty-category mappings

X) Other (please describe after [Answer]: tag below)

[Answer]: A
