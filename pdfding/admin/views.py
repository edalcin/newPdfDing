from django.conf import settings
from django.contrib import messages
from django.contrib.auth.models import User
from django.db.models.functions import Lower
from django.http import Http404
from django.shortcuts import redirect, render
from django.utils.translation import gettext_lazy as _
from django.views import View
from pdf import forms
from pdf.models.pdf_models import Pdf
from pdf.models.shared_models import SharedPdf
from pdf.models.tag_models import Tag
from pdf.services.tag_services import TagServices
from pdf.services.workspace_services import get_pdfs_of_workspace


class AdminRequiredMixin:
    """Restrict access to superuser+staff. 404 otherwise."""

    def dispatch(self, request, *args, **kwargs):
        if not (request.user.is_superuser and request.user.is_staff):
            raise Http404
        return super().dispatch(request, *args, **kwargs)


def _get_workspace_tag(request, tag_id: str):
    """Return tag with tag_id in user's current workspace, or None."""
    return request.user.profile.current_workspace.tag_set.filter(id=tag_id).first()


class Information(AdminRequiredMixin, View):
    """View for getting instance information"""

    def get(self, request):
        context = {
            'current_version': getattr(settings, 'VERSION', 'unknown'),
            'number_of_users': User.objects.count(),
            'number_of_pdfs': Pdf.objects.count(),
        }
        return render(request, 'information.html', context=context)


class TagOverview(AdminRequiredMixin, View):
    def get(self, request):
        tags = request.user.profile.current_workspace.tag_set.order_by(Lower('name'))
        return render(request, 'tag_management.html', {'tags': tags})


class CreateTag(AdminRequiredMixin, View):
    def post(self, request):
        workspace = request.user.profile.current_workspace
        form = forms.TagNameForm(request.POST)
        if form.is_valid():
            name = form.cleaned_data['name']
            if workspace.tag_set.filter(name__iexact=name).exists():
                messages.warning(request, _('A tag with this name already exists!'))
            else:
                Tag.objects.create(name=name, workspace=workspace)
        else:
            messages.warning(request, _('Input is not valid!'))
        return redirect('admin_tag_overview')


class RenameTag(AdminRequiredMixin, View):
    def post(self, request):
        workspace = request.user.profile.current_workspace
        tag = _get_workspace_tag(request, request.POST.get('tag_id', ''))
        form = forms.TagNameForm(request.POST)
        if not tag:
            messages.warning(request, _('Tag not found!'))
        elif not form.is_valid():
            messages.warning(request, _('Input is not valid!'))
        else:
            new_name = form.cleaned_data['name']
            collision = workspace.tag_set.filter(name__iexact=new_name).exclude(id=tag.id).first()
            if collision:
                messages.warning(request, _('A tag with this name already exists! Use substitute to merge tags.'))
            else:
                tag.name = new_name
                tag.save()
        return redirect('admin_tag_overview')


class DeleteTag(AdminRequiredMixin, View):
    def post(self, request):
        tag = _get_workspace_tag(request, request.POST.get('tag_id', ''))
        if tag:
            tag.delete()
        else:
            messages.warning(request, _('Tag not found!'))
        return redirect('admin_tag_overview')


class SubstituteTag(AdminRequiredMixin, View):
    def post(self, request):
        source = _get_workspace_tag(request, request.POST.get('source_id', ''))
        target = _get_workspace_tag(request, request.POST.get('target_id', ''))
        if not source or not target:
            messages.warning(request, _('Tag not found!'))
        elif source.id == target.id:
            messages.warning(request, _('Source and target tags must be different!'))
        else:
            TagServices.substitute_tag(source, target)
        return redirect('admin_tag_overview')


class SharedPdfOverview(AdminRequiredMixin, View):
    def get(self, request):
        workspace = request.user.profile.current_workspace
        shares = SharedPdf.objects.filter(pdf__in=get_pdfs_of_workspace(workspace)).order_by('-creation_date')
        return render(request, 'shared_pdf_management.html', {'shares': shares})


class RevokeShare(AdminRequiredMixin, View):
    def post(self, request):
        workspace = request.user.profile.current_workspace
        share = SharedPdf.objects.filter(
            id=request.POST.get('share_id', ''), pdf__in=get_pdfs_of_workspace(workspace)
        ).first()
        if share:
            share.delete()
        else:
            messages.warning(request, _('Share not found!'))
        return redirect('admin_shared_overview')
