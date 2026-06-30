from shutil import rmtree
from uuid import uuid4

from django.contrib.auth.models import User
from django.db import models
from django.utils.translation import gettext_lazy as _
from pdf.models.helpers import get_workspace_path


def get_uuid4_str() -> str:
    return str(uuid4())


class WorkspaceError(Exception):
    """Exceptions for workspace related problems"""


class WorkspaceRoles(models.TextChoices):
    OWNER = 'Owner'
    ADMIN = 'Admin'
    MEMBER = 'Member'
    GUEST = 'Guest'


class Workspace(models.Model):
    """The workspace model. Workspaces are the top level hierarchy."""

    id = models.CharField(primary_key=True, default=get_uuid4_str, max_length=36, editable=False, blank=False)
    creation_date = models.DateTimeField(blank=False, editable=False, auto_now_add=True)
    description = models.TextField(default='', blank=True)
    name = models.CharField(max_length=50, blank=False)
    personal_workspace = models.BooleanField(blank=False, editable=False)

    def __str__(self):  # pragma: no cover
        return str(self.name)

    def delete(self, *args, **kwargs) -> None:
        """
        Override default delete method so that workspace directory gets deleted after the workspace is deleted.
        """

        ws_path = get_workspace_path(self)
        super().delete(*args, **kwargs)

        try:
            rmtree(ws_path)
        except Exception:  # pragma: no cover # nosec B110
            pass

    @property
    def collections(self) -> models.QuerySet:
        """Get the collections of the workspace."""

        return self.collection_set.all()


class WorkspaceUser(models.Model):
    """
    The workspace user model. It is linked to both a workspace and a user profile.
    Workspace users can have the roles owner, admin, member and guest.
    """

    id = models.UUIDField(primary_key=True, default=uuid4, editable=False, blank=False)
    workspace = models.ForeignKey(Workspace, on_delete=models.CASCADE, blank=False)
    user = models.ForeignKey(User, on_delete=models.CASCADE, blank=False)
    role = models.CharField(choices=WorkspaceRoles.choices, max_length=6, blank=False)
