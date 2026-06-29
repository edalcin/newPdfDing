from base import base_views
from django.http import HttpRequest
from django.shortcuts import render
from pdf.models.collection_models import Collection
from pdf.models.workspace_models import Workspace
from pdf.services.pdf_services import check_object_access_allowed
from pdf.services.workspace_services import get_pdfs_of_workspace


class BaseWorkspaceMixin:
    obj_name = 'workspace'


class WorkspaceMixin(BaseWorkspaceMixin):
    @staticmethod
    @check_object_access_allowed
    def get_object(request: HttpRequest, ws_id: str) -> Workspace:
        """Get the current workspace."""

        user_profile = request.user.profile
        ws = user_profile.workspaces.get(id=ws_id)

        return ws


class CollectionDetails(WorkspaceMixin, base_views.BaseDetails):
    """View for displaying the collections page of a workspace."""

    def get(self, request: HttpRequest, identifier: str):
        """Display the collection page."""

        workspace = request.user.profile.current_workspace
        collection = self.get_collection(request, identifier=identifier)
        context = {
            'workspace': workspace,
            'current_collection_id': identifier,
            'collection': collection,
            'current_collection_name': collection.name,
        }

        return render(request, 'collection_details.html', context)

    @staticmethod
    @check_object_access_allowed
    def get_collection(request: HttpRequest, identifier: str) -> Collection:
        collection = request.user.profile.collections.get(id=identifier)

        return collection
