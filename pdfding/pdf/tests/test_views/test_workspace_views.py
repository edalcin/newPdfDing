from django.contrib.auth.models import User
from django.test import Client, TestCase
from django.urls import reverse
from pdf.views import workspace_views


class WorkspaceTestCase(TestCase):
    username = 'user'
    password = '12345'

    def setUp(self):
        self.client = Client()
        self.user = User.objects.create_user(username=self.username, password=self.password, email='a@a.com')
        self.client.login(username=self.username, password=self.password)


class TestWorkspaceMixin(WorkspaceTestCase):
    def test_get_object(self):
        # we need to create a request so get_pdf can access the user profile
        response = self.client.get(reverse('pdf_overview'))

        ws = self.user.profile.current_workspace

        assert ws == workspace_views.WorkspaceMixin.get_object(response.wsgi_request, ws.id)


class TestCollectionDetails(WorkspaceTestCase):
    def test_get(self):
        default_collection = self.user.profile.current_collection

        response = self.client.get(reverse('collection_details', kwargs={'identifier': default_collection.id}))

        self.assertTemplateUsed(response, 'collection_details.html')

        assert response.context['workspace'] == self.user.profile.current_workspace
        assert response.context['collection'] == default_collection
        assert response.context['current_collection_id'] == default_collection.id
        assert response.context['current_collection_name'] == default_collection.name
