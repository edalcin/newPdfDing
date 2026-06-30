from unittest.mock import patch

from django.contrib.auth.models import User
from django.test import TestCase
from pdf.models.pdf_models import Pdf
from pdf.models.tag_models import Tag


class TestSignals(TestCase):
    def test_delete_orphan_tag(self):
        # create pdfs and tags
        user = User.objects.create_user(username='test_user', password='12345')
        pdf_1 = Pdf.objects.create(collection=user.profile.current_collection, name='pdf_1')
        pdf_2 = Pdf.objects.create(collection=user.profile.current_collection, name='pdf_2')
        tag_1 = Tag.objects.create(name='tag_1', workspace=user.profile.current_workspace)
        tag_2 = Tag.objects.create(name='tag_2', workspace=user.profile.current_workspace)
        pdf_1.tags.set([tag_1, tag_2])
        pdf_2.tags.set([tag_2])

        pdf_1.delete()

        # check that the tag of pdf 2 was not touched
        pdf_2_tag_names = [tag.name for tag in pdf_2.tags.all()]
        self.assertEqual(pdf_2_tag_names, ['tag_2'])

        # check that tag 1 was deleted
        self.assertFalse(user.profile.tags.filter(name='tag_1').exists())

    @patch('pdf.signals.create_personal_workspace')
    def test_create_workspace(self, mock_create_personal_workspace):
        user = User.objects.create_user(username='test_user', password='12345')

        mock_create_personal_workspace.assert_called_once_with(user)
