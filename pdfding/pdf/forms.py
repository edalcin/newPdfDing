import re

import magic
from django import forms
from django.core.exceptions import ValidationError
from django.core.files import File
from django.db.models import QuerySet
from django.utils.translation import gettext_lazy as _
from pdf.models.collection_models import Collection
from pdf.models.pdf_models import Pdf
from pdf.services.pdf_services import compute_file_sha256
from pdf.services.workspace_services import check_if_pdf_with_hash_exists, check_if_pdf_with_name_exists


class AddFormNoFile(forms.ModelForm):
    """Class for creating the form for adding PDFs in the demo mode."""

    tag_string = forms.CharField(
        required=False,
        widget=forms.TextInput(attrs={'class': 'form-control', 'placeholder': _('Add Tags')}),
        help_text=_(
            'Optional, enter any number of tags separated by space and without the hashtag (#). '
            'If a tag does not exist it will be automatically created.'
        ),
    )

    use_file_name = forms.BooleanField(required=False, widget=forms.CheckboxInput(attrs={'class': 'form-control'}))

    class Meta:
        model = Pdf
        widgets = {
            'name': forms.TextInput(attrs={'placeholder': _('Add PDF Name')}),
            'description': forms.Textarea(attrs={'rows': 2, 'placeholder': _('Add Description')}),
            'notes': forms.Textarea(attrs={'rows': 8, 'placeholder': _('Add Notes')}),
            'file_directory': forms.TextInput(attrs={'placeholder': _('Add File Directory')}),
        }

        fields = ['name', 'description', 'notes', 'file_directory']

    def __init__(self, *args, **kwargs):
        """
        Adds the profile to the form. This is done, so we can access information about the profile
        when creating a new pdf.
        """

        self.profile = kwargs.pop('profile', None)
        if not self.profile:
            raise KeyError('profile')

        super(AddFormNoFile, self).__init__(*args, **kwargs)
        self.fields['collection'] = get_collection_choices(
            self.profile.current_collection_id, self.profile.current_collection_name, self.profile.collections
        )

    def clean_name(self) -> str:
        """
        Clean the submitted pdf name. Removes trailing and multiple whitespaces. Also checks if the user already
        has an uploaded PDF with the same name.
        """

        pdf_name = CleanHelpers.clean_name(self.cleaned_data['name'])

        current_workspace = self.profile.current_workspace
        existing_pdf = check_if_pdf_with_name_exists(pdf_name, current_workspace)

        # only raise validation error if name is not the dummy placeholder from the frontend
        # otherwise it will be replaced by the filename in "clean".
        if pdf_name != 'bb36974a-3792-47c5-96cc-c79adb87cf82' and existing_pdf:
            raise forms.ValidationError(_('A PDF with this name already exists!'))

        return pdf_name

    def clean_tag_string(self) -> str:  # pragma: no cover
        return CleanHelpers.clean_tag_string_file_directory(self.cleaned_data['tag_string'])

    def clean_file_directory(self) -> str:  # pragma: no cover
        return CleanHelpers.clean_file_directory(self.cleaned_data['file_directory'])


class AddForm(AddFormNoFile):
    """Class for creating the form for adding PDFs."""

    class Meta(AddFormNoFile.Meta):
        model = Pdf
        widgets = AddFormNoFile.Meta.widgets
        widgets['file'] = forms.ClearableFileInput(attrs={'accept': 'application/pdf'})

        fields = AddFormNoFile.Meta.fields
        fields.append('file')

    def clean_file(self) -> File:  # pragma: no cover
        """Clean the submitted pdf file. Checks if the file is a pdf and not a duplicate."""
        file = CleanHelpers.clean_file(self.cleaned_data['file'])
        sha256 = compute_file_sha256(file)
        existing = check_if_pdf_with_hash_exists(sha256, self.profile.current_workspace)
        if existing:
            raise forms.ValidationError(
                _('This PDF already exists in your library (as \'%(name)s\').') % {'name': existing.name}
            )
        return file


class MultipleFileInput(forms.ClearableFileInput):  # pragma: no cover
    allow_multiple_selected = True


class MultipleFileField(forms.FileField):  # pragma: no cover
    def __init__(self, *args, **kwargs):
        kwargs.setdefault("widget", MultipleFileInput())
        super().__init__(*args, **kwargs)

    def clean(self, data, initial=None):
        single_file_clean = super().clean
        if isinstance(data, (list, tuple)):
            result = [single_file_clean(d, initial) for d in data]
        else:
            result = [single_file_clean(data, initial)]
        return result


class BulkAddFormNoFile(forms.Form):
    """Class for creating the form for bulk adding PDFs in the demo mode."""

    description = forms.CharField(
        required=False,
        widget=forms.Textarea(attrs={'rows': 3, 'placeholder': _('Add Description')}),
        help_text=_('Optional'),
    )

    notes = forms.CharField(
        required=False,
        widget=forms.Textarea(attrs={'rows': 8, 'placeholder': _('Add Notes')}),
        help_text=_('Optional, supports Markdown'),
    )

    tag_string = forms.CharField(
        required=False,
        widget=forms.TextInput(attrs={'class': 'form-control', 'placeholder': _('Add Tags')}),
        help_text=_(
            'Optional, enter any number of tags separated by space and without the hashtag (#). '
            'If a tag does not exist it will be automatically created.'
        ),
    )

    file_directory = forms.CharField(
        required=False,
        widget=forms.TextInput(attrs={'class': 'form-control', 'placeholder': _('Add File Directory')}),
        help_text=_('Optional, save file in a sub directory of the pdf directory, e.g: important/pdfs'),
    )

    def __init__(self, *args, **kwargs):
        """
        Adds the profile to the form. This is done, so we can access information about the profile
        when creating a new pdf.
        """

        self.profile = kwargs.pop('profile', None)
        if not self.profile:
            raise KeyError('profile')

        super(BulkAddFormNoFile, self).__init__(*args, **kwargs)
        self.fields['collection'] = get_collection_choices(
            self.profile.current_collection_id, self.profile.current_collection_name, self.profile.collections
        )

    def clean_file(self):
        for file in self.cleaned_data['file']:
            CleanHelpers.clean_file(file)

    def clean_tag_string(self) -> str:  # pragma: no cover
        return CleanHelpers.clean_tag_string_file_directory(self.cleaned_data['tag_string'])

    def clean_file_directory(self) -> str:  # pragma: no cover
        return CleanHelpers.clean_file_directory(self.cleaned_data['file_directory'])


class BulkAddForm(BulkAddFormNoFile):
    """Class for creating the form for bulk adding PDFs."""

    file = MultipleFileField(
        required=True,
        widget=MultipleFileInput(attrs={'accept': 'application/pdf'}),
    )


class DescriptionForm(forms.ModelForm):
    """Form for changing the description of a PDF."""

    class Meta:
        model = Pdf
        widgets = {'description': forms.Textarea(attrs={'rows': 3})}
        fields = ['description']


class NotesForm(forms.ModelForm):
    """Form for changing the notes of a PDF."""

    class Meta:
        model = Pdf
        widgets = {'notes': forms.Textarea(attrs={'rows': 20})}
        fields = ['notes']


class NameForm(forms.ModelForm):
    """Form for changing the name of a PDF."""

    class Meta:
        model = Pdf
        fields = ['name']

    def clean_name(self) -> str:  # pragma: no cover
        """Clean the submitted pdf name. Removes trailing and multiple whitespaces."""

        pdf_name = CleanHelpers.clean_name(self.cleaned_data['name'])

        return pdf_name


class FileDirectoryForm(forms.ModelForm):
    """Form for changing the directory of a PDF."""

    class Meta:
        model = Pdf
        fields = ['file_directory']

    def clean_file_directory(self) -> str:  # pragma: no cover
        """Clean the submitted pdf file directory name."""

        file_directory = self.cleaned_data['file_directory']

        return CleanHelpers.clean_file_directory(file_directory)


class PdfTagsForm(forms.ModelForm):
    """Form for changing the tags of a PDF."""

    tag_string = forms.CharField(widget=forms.TextInput(), required=False)

    class Meta:
        model = Pdf
        fields = []

    def clean_tag_string(self) -> str:  # pragma: no cover
        return CleanHelpers.clean_tag_string_file_directory(self.cleaned_data['tag_string'])


class PdfCollectionForm(forms.ModelForm):
    """Form for changing the collection of a PDF."""

    class Meta:
        model = Pdf
        fields = []

    def __init__(self, *args, **kwargs):  # pragma: no cover
        """
        Adds the profile to the form. This is done, so we can access information about the profile
        when creating a new pdf.
        """

        pdf = kwargs['instance']
        if not pdf:
            raise KeyError('instance')

        super(forms.ModelForm, self).__init__(*args, **kwargs)

        self.fields['collection'] = get_collection_choices(
            pdf.collection.id, pdf.collection.name, pdf.collection.workspace.collection_set.all()
        )


class TagNameForm(forms.Form):
    """Form for changing the name of a tag."""

    name = forms.CharField(
        required=True,
        widget=forms.TextInput(attrs={'class': 'form-control'}),
    )

    def clean_name(self) -> str:
        new_tag_name = self.cleaned_data['name'].strip()

        if re.search(r'\s', new_tag_name):
            raise ValidationError(_('Tag names are not allowed to contain spaces!'))

        new_tag_name = CleanHelpers.clean_tag_string_file_directory(new_tag_name)

        return new_tag_name


class CollectionForm(forms.Form):
    """Class for creating the form for creating collections."""

    name = forms.CharField(
        required=True,
        widget=forms.TextInput(attrs={'class': 'form-control', 'placeholder': _('Add Collection Name')}),
    )
    description = forms.CharField(
        required=False,
        widget=forms.Textarea(attrs={'class': 'form-control', 'placeholder': _('Add a collection description')}),
        help_text=_('Optional'),
    )

    def __init__(self, *args, **kwargs):
        """
        Adds the profile to the form. This is done, so we can access information about the profile
        when creating a new collection.
        """

        self.profile = kwargs.pop('profile', None)
        if not self.profile:
            raise KeyError('profile')

        super(CollectionForm, self).__init__(*args, **kwargs)

    def clean_name(self) -> str:
        """
        Clean the submitted collection name. Removes trailing and multiple whitespaces. Checks that only
        numbers, letters, '_' and '-' are used. Also checks if a collection with the same name already
        exists in the workspace.
        """

        workspace = self.profile.current_workspace
        collection_name = CleanHelpers.clean_workspace_name(self.cleaned_data['name'])

        if collection_name.lower() == 'all':
            raise ValidationError(_('"All" is not a valid collection name!'))

        if workspace.collections.filter(name__iexact=collection_name).count():
            raise ValidationError(
                _('There is already a collection named {collection_name} in the current workspace!').format(
                    collection_name=collection_name
                )
            )

        return collection_name


class CollectionNameForm(forms.ModelForm):
    """Form for changing the name of a collection."""

    class Meta:
        model = Collection
        fields = ['name']

    def clean_name(self) -> str:  # pragma: no cover
        """Clean the submitted collection name. Removes trailing and multiple whitespaces."""

        collection_name = CleanHelpers.clean_workspace_name(self.cleaned_data['name'])

        return collection_name


class CollectionDescriptionForm(forms.ModelForm):
    """Form for changing the description of a collection."""

    class Meta:
        model = Collection
        widgets = {'description': forms.Textarea(attrs={'rows': 3})}
        fields = ['description']


class CleanHelpers:
    @staticmethod
    def clean_file(file: File) -> File:
        """Clean the submitted pdf file. Checks if the file is a pdf."""

        # recommended to use at least the first 2048 bytes, as less can produce incorrect identification
        file_type = magic.from_buffer(file.read(2048), mime=True)

        if file_type.lower() != 'application/pdf':
            raise forms.ValidationError(_('Uploaded file is not a PDF!'))

        return file

    @staticmethod
    def clean_name(name: str) -> str:
        """Clean the submitted name. Removes trailing and multiple whitespaces."""

        name.strip()
        name = ' '.join(name.split())

        return name

    @classmethod
    def clean_file_directory(cls, file_directory: str) -> str:  # pragma: no cover
        """
        Clean the submitted file directory. Removes trailing, multiple whitespaces. Raises ValidationError for not
        allowed chars. Allowed are only alphanumeric chars and those in ['/', '-', '_', space].
        """

        if file_directory:
            file_directory = file_directory.strip()

            if re.search(r'\s', file_directory):
                raise ValidationError(_('Directories are not allowed to contain spaces!'))

            file_directory = cls.clean_tag_string_file_directory(file_directory)

        return file_directory

    @staticmethod
    def clean_tag_string_file_directory(input_string: str):
        """
        Clean the input string. Allowed are only alphanumeric chars and those in ['/', '-', '_', space].
        """

        if input_string:
            for char in input_string:
                if not (char.isalnum() or char in ['/', '-', '_', ' ']):
                    raise forms.ValidationError(_('Only letters, numbers, "/", "-" and "_" are valid characters!'))

            string_split = input_string.split(' ')

            for string_part in string_split:
                # only process non-empty strings
                # if a string has multiple spaces between this would cause a IndexError otherwise.
                if string_part:
                    string_part = string_part.strip()

                    if string_part[0] == '/' or string_part[-1] == '/':
                        raise forms.ValidationError(_('Not allowed to begin or end with "/"!'))

                    if re.search(r'/{2,}', string_part):
                        raise forms.ValidationError(_('Not allowed to contain consecutive "/" characters!'))

        return input_string

    @staticmethod
    def clean_workspace_name(ws_name: str) -> str:
        """
        Clean the submitted workspace name. Removes trailing and multiple whitespaces. Checks that only
        numbers, letters, '_' and '-' are used.
        """

        ws_name = ws_name.strip()

        if ws_name in ['_', '-']:
            raise forms.ValidationError(_('"_" or "-" are not valid workspace names!'))
        elif ws_name and not re.match(r'^[A-Za-z0-9-_]*$', ws_name):
            raise forms.ValidationError(_('Only "-", "_", numbers or letters are allowed!'))
        if len(ws_name) > 50:
            raise forms.ValidationError(_('Maximum number of characters for a workspace name is 50!'))

        return ws_name


def get_collection_choices(
    current_collection_id: str, current_collection_name: str, collections: QuerySet[Collection]
) -> forms.ChoiceField:
    choices = []

    # make sure current collection is first collection in dropdown
    if current_collection_id != 'all':
        choices.append((current_collection_id, current_collection_name))

    for collection in collections:
        if collection.id != current_collection_id:
            choices.append((collection.id, collection.name))

    return forms.ChoiceField(choices=choices)
