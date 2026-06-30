# Graph Report - newPdfDing  (2026-06-30)

## Corpus Check
- 180 files · ~134,369 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 489 nodes · 661 edges · 37 communities (32 shown, 5 thin omitted)
- Extraction: 92% EXTRACTED · 8% INFERRED · 0% AMBIGUOUS · INFERRED: 50 edges (avg confidence: 0.61)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `d02fb0b2`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- [[_COMMUNITY_Community 0|Community 0]]
- [[_COMMUNITY_Community 1|Community 1]]
- [[_COMMUNITY_Community 2|Community 2]]
- [[_COMMUNITY_Community 3|Community 3]]
- [[_COMMUNITY_Community 4|Community 4]]
- [[_COMMUNITY_Community 5|Community 5]]
- [[_COMMUNITY_Community 6|Community 6]]
- [[_COMMUNITY_Community 7|Community 7]]
- [[_COMMUNITY_Community 8|Community 8]]
- [[_COMMUNITY_Community 9|Community 9]]
- [[_COMMUNITY_Community 10|Community 10]]
- [[_COMMUNITY_Community 11|Community 11]]
- [[_COMMUNITY_Community 12|Community 12]]
- [[_COMMUNITY_Community 13|Community 13]]
- [[_COMMUNITY_Community 14|Community 14]]
- [[_COMMUNITY_Community 15|Community 15]]
- [[_COMMUNITY_Community 16|Community 16]]
- [[_COMMUNITY_Community 17|Community 17]]
- [[_COMMUNITY_Community 18|Community 18]]
- [[_COMMUNITY_Community 19|Community 19]]
- [[_COMMUNITY_Community 20|Community 20]]
- [[_COMMUNITY_Community 21|Community 21]]
- [[_COMMUNITY_Community 22|Community 22]]
- [[_COMMUNITY_Community 23|Community 23]]
- [[_COMMUNITY_Community 24|Community 24]]
- [[_COMMUNITY_Community 25|Community 25]]
- [[_COMMUNITY_Community 26|Community 26]]
- [[_COMMUNITY_Community 27|Community 27]]
- [[_COMMUNITY_Community 28|Community 28]]
- [[_COMMUNITY_Community 29|Community 29]]
- [[_COMMUNITY_Community 30|Community 30]]
- [[_COMMUNITY_Community 31|Community 31]]
- [[_COMMUNITY_Community 32|Community 32]]
- [[_COMMUNITY_Community 33|Community 33]]
- [[_COMMUNITY_Community 34|Community 34]]
- [[_COMMUNITY_Community 35|Community 35]]
- [[_COMMUNITY_Community 36|Community 36]]

## God Nodes (most connected - your core abstractions)
1. `Profile` - 32 edges
2. `TestProfileSettingsViews` - 28 edges
3. `TestCleanHelpers` - 23 edges
4. `Workspace` - 22 edges
5. `TestProfile` - 17 edges
6. `TagE2ETestCase` - 16 edges
7. `create_collection()` - 16 edges
8. `TestWorkspaceServices` - 13 edges
9. `CleanHelpers` - 12 edges
10. `TestPdfForms` - 12 edges

## Surprising Connections (you probably didn't know these)
- `TestCleanHelpers` --uses--> `CleanHelpers`  [INFERRED]
  pdfding/pdf/tests/test_forms.py → pdfding/pdf/forms.py
- `create_collection()` --calls--> `WorkspaceError`  [INFERRED]
  pdfding/pdf/services/workspace_services.py → pdfding/pdf/models/workspace_models.py
- `create_personal_workspace()` --calls--> `WorkspaceError`  [INFERRED]
  pdfding/pdf/services/workspace_services.py → pdfding/pdf/models/workspace_models.py
- `TestMigrations` --uses--> `Workspace`  [INFERRED]
  pdfding/pdf/tests/test_migrations.py → pdfding/pdf/models/workspace_models.py
- `AnnotationsSortingChoice` --uses--> `Workspace`  [INFERRED]
  pdfding/users/models.py → pdfding/pdf/models/workspace_models.py

## Import Cycles
- None detected.

## Communities (37 total, 5 thin omitted)

### Community 0 - "Community 0"
Cohesion: 0.05
Nodes (14): CleanHelpers, NameForm, Form for changing the name of a PDF., Clean the submitted pdf name. Removes trailing and multiple whitespaces., Clean the submitted name. Removes trailing and multiple whitespaces., TestCase, TestCollectionForms, TestOther (+6 more)

### Community 1 - "Community 1"
Cohesion: 0.08
Nodes (30): The workspace model. Workspaces are the top level hierarchy., Override default delete method so that workspace directory gets deleted after th, Workspace, create_workspace(), delete_orphan_tag(), handle_workspaces_after_user_delete(), Create the personal workspace when a user is created., Delete all workspaces belonging to the user. (+22 more)

### Community 2 - "Community 2"
Cohesion: 0.06
Nodes (3): BaseProfileView, TestProfileOtherViews, TestProfileSettingsViews

### Community 3 - "Community 3"
Cohesion: 0.07
Nodes (17): Get the collections of the workspace., QuerySet, Profile, Return dark mode property so that it can be used in templates., Return the current workspace associated of the profile., Return the current collection of the profile., Return the name of the current collection, Return all PDFs of all workspaces the user has access to. (+9 more)

### Community 4 - "Community 4"
Cohesion: 0.07
Nodes (5): create_collection(), Create a collection and add it to the workspace, TestWorkspace, TestCollectionServices, TestProfile

### Community 5 - "Community 5"
Cohesion: 0.09
Nodes (15): add_file_to_minio(), backup_function(), backup_sqlite(), backup_task(), check_backup_requirements(), difference_local_minio(), Compare the local PDF and qr code files to the files in the minio bucket.      R, Add a file to minio. If an encryption key is provided the file will be encrypted (+7 more)

### Community 7 - "Community 7"
Cohesion: 0.12
Nodes (12): TestMigrations, convert_hex_to_rgb(), convert_rgb_to_hex(), darken_color(), get_demo_pdf(), get_secondary_color(), get_viewer_theme_and_color(), Converts RGB color representation to a hex representation (+4 more)

### Community 8 - "Community 8"
Cohesion: 0.10
Nodes (7): Exception, Exceptions for workspace related problems, The workspace user model. It is linked to both a workspace and a user profile., WorkspaceError, WorkspaceRoles, WorkspaceUser, TestWorkspaceServices

### Community 9 - "Community 9"
Cohesion: 0.15
Nodes (13): Collection, Pdf, adjust_pdf_path(), change_collection_of_pdf(), move_collection(), move_collection_file(), Change the collection of a PDF., Adjust path of PDF when the path of a collection is changed (+5 more)

### Community 11 - "Community 11"
Cohesion: 0.18
Nodes (10): BaseDelete, BaseDetailsEdit, The base view for editing fields of an object in the details page. The field, th, Triggered by htmx. Display an inline form for editing the correct field., POST: Change the specified field by submitting the form., Base view for deleting the object specified by its ID., Delete the specified object., Execute before deleting object. (+2 more)

### Community 12 - "Community 12"
Cohesion: 0.15
Nodes (11): account_settings(), ChangeTreeMode, danger_settings(), View for turning tag tree mode on and off., Change the sorting setting., View for the account settings page, View for the ui settings page, View for the viewer settings page (+3 more)

### Community 13 - "Community 13"
Cohesion: 0.20
Nodes (8): ChoiceField, get_collection_choices(), PdfCollectionForm, Adds the profile to the form. This is done, so we can access information about t, Form for changing the collection of a PDF., Adds the profile to the form. This is done, so we can access information about t, Adds the profile to the form. This is done, so we can access information about t, Adds the profile to the form. This is done, so we can access information about t

### Community 14 - "Community 14"
Cohesion: 0.18
Nodes (9): CollectionDescriptionForm, DescriptionForm, Meta, MultipleFileField, MultipleFileInput, NotesForm, Form for changing the description of a PDF., Form for changing the notes of a PDF. (+1 more)

### Community 15 - "Community 15"
Cohesion: 0.18
Nodes (9): BaseOverviewQuery, Base view for performing searches in the overview pages., ChangeLayout, ChangeSorting, View for changing the sorting settings for the overviews, Change the sorting setting., View for changing the layout settings for the pdf overview, Change the layout setting. (+1 more)

### Community 16 - "Community 16"
Cohesion: 0.22
Nodes (6): BaseDetails, BaseServe, Base view used for serving PDF files specified by the PDF id., Returns the specified file as a FileResponse          There is a revision parame, Base view for displaying the details page of an object., Display the details page.

### Community 17 - "Community 17"
Cohesion: 0.20
Nodes (7): CollectionForm, CollectionNameForm, Class for creating the form for creating collections., Clean the submitted collection name. Removes trailing and multiple whitespaces., Form for changing the name of a collection., Clean the submitted collection name. Removes trailing and multiple whitespaces., Clean the submitted workspace name. Removes trailing and multiple whitespaces. C

### Community 18 - "Community 18"
Cohesion: 0.22
Nodes (5): PdfTagsForm, Form for changing the tags of a PDF., Form for changing the name of a tag., Clean the input string. Allowed are only alphanumeric chars and those in ['/', ', TagNameForm

### Community 19 - "Community 19"
Cohesion: 0.25
Nodes (4): AdminLoginView, View for gettings and setting signatures, Single-admin password login view., Signatures

### Community 20 - "Community 20"
Cohesion: 0.38
Nodes (4): BaseOverview, Base view for the overview pages. This view performs the searching and sorting., Display the overview., Do some action before rendering the overview

### Community 21 - "Community 21"
Cohesion: 0.29
Nodes (5): AddForm, AddFormNoFile, Class for creating the form for adding PDFs in the demo mode., Clean the submitted pdf name. Removes trailing and multiple whitespaces. Also ch, Class for creating the form for adding PDFs.

### Community 22 - "Community 22"
Cohesion: 0.29
Nodes (4): FileDirectoryForm, Form for changing the directory of a PDF., Clean the submitted pdf file directory name., Clean the submitted file directory. Removes trailing, multiple whitespaces. Rais

### Community 23 - "Community 23"
Cohesion: 0.33
Nodes (4): BaseAdd, View for adding new objects., Display the form for adding an object., Create the new object.

### Community 24 - "Community 24"
Cohesion: 0.40
Nodes (4): BaseDownload, Base view for downloading the PDF specified by the ID., Get an empty suffix. This is currently used for pdf files as otherwise download, Return the specified file as a FileResponse.

### Community 25 - "Community 25"
Cohesion: 0.40
Nodes (3): File, Clean the submitted pdf file. Checks if the file is a pdf., Clean the submitted pdf file. Checks if the file is a pdf and not a duplicate.

### Community 26 - "Community 26"
Cohesion: 0.33
Nodes (4): ChangeSetting, View for changing the settings., For a htmx request this will load a change pdfs per page form as a partial, Process the submitted change settings form

### Community 27 - "Community 27"
Cohesion: 0.40
Nodes (4): BulkAddForm, BulkAddFormNoFile, Class for creating the form for bulk adding PDFs in the demo mode., Class for creating the form for bulk adding PDFs.

### Community 28 - "Community 28"
Cohesion: 0.40
Nodes (3): Delete, View for deleting a user profile., Display the page for deleting the user

### Community 30 - "Community 30"
Cohesion: 0.50
Nodes (3): get_qrcode_file_path(), Migration, Inline: was in shared_models.py, now deleted.

### Community 31 - "Community 31"
Cohesion: 0.50
Nodes (3): get_collection_qr_code_path(), Migration, Inline: was in shared_models.py, now deleted.

### Community 32 - "Community 32"
Cohesion: 0.50
Nodes (3): ChangeCollection, View for changing the current collection., Change the current workspace.

### Community 33 - "Community 33"
Cohesion: 0.50
Nodes (3): OpenCollapseTags, View for opening and collapsing tags in the pdf overview, Open or collapse the tags in the pdf overview

### Community 34 - "Community 34"
Cohesion: 0.50
Nodes (3): View for updating the last time a user was nagged., Update the last time a user was nagged with the current datetime., UpdateLastTimeNagged

## Knowledge Gaps
- **6 isolated node(s):** `Meta`, `Migration`, `Migration`, `Migration`, `WorkspaceRoles` (+1 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Profile` connect `Community 3` to `Community 0`, `Community 1`, `Community 2`, `Community 32`, `Community 33`, `Community 34`, `Community 7`, `Community 12`, `Community 15`, `Community 19`, `Community 26`, `Community 28`?**
  _High betweenness centrality (0.301) - this node is a cross-community bridge._
- **Why does `Workspace` connect `Community 1` to `Community 8`, `Community 3`, `Community 4`, `Community 7`?**
  _High betweenness centrality (0.187) - this node is a cross-community bridge._
- **Why does `create_collection()` connect `Community 4` to `Community 8`, `Community 1`, `Community 9`, `Community 0`?**
  _High betweenness centrality (0.108) - this node is a cross-community bridge._
- **Are the 15 inferred relationships involving `Profile` (e.g. with `BaseProfileView` and `TestAuthRelated`) actually correct?**
  _`Profile` has 15 INFERRED edges - model-reasoned connections that need verification._
- **Are the 10 inferred relationships involving `Workspace` (e.g. with `TestMigrations` and `AnnotationsSortingChoice`) actually correct?**
  _`Workspace` has 10 INFERRED edges - model-reasoned connections that need verification._
- **What connects `Periodic huey task for backing up the PDF and QR code files and (if used) the sq`, `If at least one user and one PDF are present in the database backup requirements`, `Function for backing up the PDF and QR code files and (if used) the sqlite datab` to the rest of the system?**
  _127 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Community 0` be split into smaller, more focused modules?**
  _Cohesion score 0.04875886524822695 - nodes in this community are weakly interconnected._