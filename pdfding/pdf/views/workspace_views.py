from base import base_views
from django.http import HttpRequest
from django.shortcuts import render
from pdf.models.collection_models import Collection
from pdf.services.pdf_services import check_object_access_allowed


class CollectionDetails(base_views.BaseDetails):
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
