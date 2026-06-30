from django.contrib.auth.models import User
from django.test import TestCase
from pdf.models.helpers import get_workspace_path
from pdf.services.workspace_services import create_collection, create_workspace


class TestWorkspace(TestCase):
    def setUp(self):
        self.user = User.objects.create_user(username='user_1', password='password')
        self.ws = create_workspace(name='ws', creator=self.user)
        self.other_user = User.objects.create_user(username='other_user', password='password')

    def test_delete(self):
        ws_path = get_workspace_path(self.ws)
        pdfs_path = ws_path / 'some_collection' / 'pdf'
        pdfs_path.mkdir(parents=True)

        dummy_file_path = pdfs_path / 'dummy.pdf'
        dummy_file_path.touch()

        self.ws.delete()

        self.assertFalse(ws_path.exists())

    def test_collections_property(self):
        self.assertEqual(self.ws.collections.count(), 1)

        created_collection = create_collection(self.ws, 'some_collection')

        self.assertEqual(self.ws.collections.count(), 2)

        self.assertEqual(self.ws.collections.order_by('name')[1], created_collection)
