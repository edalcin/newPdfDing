import json
from datetime import datetime, timezone

from django.conf import settings as django_settings
from django.contrib import messages
from django.contrib.auth import login as auth_login
from django.contrib.auth import logout
from django.contrib.auth.decorators import login_not_required
from django.contrib.auth.models import User
from django.http import Http404, HttpRequest, HttpResponse, JsonResponse
from django.shortcuts import redirect, render
from django.urls import reverse
from django.utils.decorators import method_decorator
from django.utils.translation import gettext_lazy as _
from django.views import View
from django_htmx.http import HttpResponseClientRefresh
from pdf.services.workspace_services import check_if_collection_part_of_workspace
from users import forms
from users.models import Profile
from users.service import get_secondary_color


def account_settings(request):
    """View for the account settings page"""

    return render(request, 'account_settings.html', {})


def ui_settings(request):  # pragma: no cover
    """View for the ui settings page"""

    # pragma: no cover
    return render(request, 'ui_settings.html')


def viewer_settings(request):  # pragma: no cover
    """View for the viewer settings page"""

    # pragma: no cover
    return render(request, 'viewer_settings.html')


def danger_settings(request):  # pragma: no cover
    """View for the danger settings page"""

    # pragma: no cover
    return render(request, 'danger_settings.html')


class ChangeSetting(View):
    """View for changing the settings."""

    form_dict = {
        'email': forms.EmailForm,
        'language': forms.create_user_field_form(['language']),
        'theme': forms.create_user_field_form(['dark_mode']),
        'theme_color': forms.create_user_field_form(['theme_color']),
        'pdf_inverted_mode': forms.create_user_field_form(['pdf_inverted_mode']),
        'pdf_keep_screen_awake': forms.create_user_field_form(['pdf_keep_screen_awake']),
        'custom_theme_color': forms.CustomThemeColorForm,
        'show_progress_bars': forms.create_user_field_form(['show_progress_bars']),
    }

    def get(self, request: HttpRequest, field_name: str):
        """For a htmx request this will load a change pdfs per page form as a partial"""

        initial_dict = {
            'email': {'email': request.user.email},
            'language': {'language': request.user.profile.language},
            'theme': {'dark_mode': request.user.profile.dark_mode},
            'theme_color': {'theme_color': request.user.profile.theme_color},
            'custom_theme_color': {'custom_theme_color': request.user.profile.custom_theme_color},
            'pdf_inverted_mode': {'pdf_inverted_mode': request.user.profile.pdf_inverted_mode},
            'pdf_keep_screen_awake': {'pdf_keep_screen_awake': request.user.profile.pdf_keep_screen_awake},
            'show_progress_bars': {'show_progress_bars': request.user.profile.show_progress_bars},
        }

        if request.htmx:
            form = self.form_dict[field_name](initial=initial_dict[field_name])

            return render(
                request,
                'partials/settings_form.html',
                {
                    'form': form,
                    'action_url': reverse('profile-setting-change', kwargs={'field_name': field_name}),
                    'edit_id': f'{field_name}_edit',
                },
            )

        return redirect('home')

    def post(self, request: HttpRequest, field_name: str):
        """Process the submitted change settings form"""

        if field_name == 'email':
            form = self.form_dict[field_name](request.POST, instance=request.user)
        else:
            form = self.form_dict[field_name](request.POST, instance=request.user.profile)

        if form.is_valid():
            if field_name == 'email':
                email = form.cleaned_data['email']
                if User.objects.filter(email=email).exclude(id=request.user.id).exists():
                    messages.warning(request, f'{email} is already in use.')
                    return redirect('account_settings')
                form.save()
            elif field_name == 'custom_theme_color':
                form.save()

                # calculate shades for custom theme colors
                profile = request.user.profile
                profile.custom_theme_color_secondary = get_secondary_color(request.user.profile.custom_theme_color)
                profile.save()
            else:
                form.save()

        else:
            try:
                messages.warning(request, dict(form.errors)[field_name][0])
            except:  # noqa # pragma: no cover
                messages.warning(request, 'Input is not valid!')

        return redirect(request.META.get('HTTP_REFERER', 'account_settings'))


class ChangeSorting(View):
    """View for changing the sorting settings for the overviews"""

    def post(self, request: HttpRequest, sorting_category: str, sorting: str):
        """Change the sorting setting."""

        if request.htmx:
            user_profile = request.user.profile

            match sorting_category:
                case 'annotation_sorting':
                    user_profile.annotation_sorting = Profile.AnnotationsSortingChoice[str.upper(sorting)]
                case 'pdf_sorting':
                    user_profile.pdf_sorting = Profile.PdfSortingChoice[str.upper(sorting)]

            user_profile.save()

            return HttpResponseClientRefresh()

        return redirect('account_settings')


class ChangeLayout(View):
    """View for changing the layout settings for the pdf overview"""

    def post(self, request: HttpRequest, layout: str):
        """Change the layout setting."""

        if request.htmx:
            user_profile = request.user.profile
            user_profile.layout = Profile.LayoutChoice[str.upper(layout)]
            user_profile.save()

            return HttpResponseClientRefresh()

        return redirect('account_settings')


class ChangeTreeMode(View):
    """View for turning tag tree mode on and off."""

    def post(self, request: HttpRequest):
        """Change the sorting setting."""

        if request.htmx:
            user_profile = request.user.profile
            user_profile.tag_tree_mode = not user_profile.tag_tree_mode

            user_profile.save()

            return HttpResponseClientRefresh()

        return redirect('account_settings')


class ChangeCollection(View):
    """View for changing the current collection."""

    def post(self, request: HttpRequest, collection_id: str):
        """Change the current workspace."""

        if request.htmx:
            user_profile = request.user.profile

            if collection_id == 'all' or check_if_collection_part_of_workspace(
                user_profile.current_workspace, collection_id
            ):
                user_profile.current_collection_id = collection_id
                user_profile.save()

                return HttpResponseClientRefresh()
            else:
                raise Http404('Collection not part of the current workspace!')

        return redirect('pdf_overview')


class OpenCollapseTags(View):
    """View for opening and collapsing tags in the pdf overview"""

    def post(self, request: HttpRequest):
        """Open or collapse the tags in the pdf overview"""

        if request.htmx:  # type: ignore
            user_profile = request.user.profile  # type: ignore
            user_profile.tags_open = not user_profile.tags_open

            user_profile.save()

            return HttpResponseClientRefresh()

        return redirect('account_settings')


class UpdateLastTimeNagged(View):
    """View for updating the last time a user was nagged."""

    def post(self, request: HttpRequest):
        """Update the last time a user was nagged with the current datetime."""

        if request.htmx:  # type: ignore
            user_profile = request.user.profile  # type: ignore
            user_profile.last_time_nagged = datetime.now(tz=timezone.utc)

            user_profile.save()

            return HttpResponseClientRefresh()

        return redirect('pdf_overview')


class Signatures(View):
    """View for gettings and setting signatures"""

    def get(self, request: HttpRequest):
        user_profile = request.user.profile  # type: ignore

        return JsonResponse(user_profile.signatures)

    def post(self, request: HttpRequest):
        user_profile = request.user.profile  # type: ignore

        viewer_current_signatures = request.POST.get('current_signatures')
        viewer_previous_signatures = request.POST.get('previous_signatures')
        viewer_current_signatures = json.loads(viewer_current_signatures)  # type: ignore
        viewer_previous_signatures = json.loads(viewer_previous_signatures)  # type: ignore

        signatures_to_be_removed = [sig for sig in viewer_previous_signatures if sig not in viewer_current_signatures]
        signatures_to_be_added = [sig for sig in viewer_current_signatures if sig not in viewer_previous_signatures]

        for sig in signatures_to_be_removed:
            user_profile.signatures.pop(sig, None)

        for sig in signatures_to_be_added:
            user_profile.signatures[sig] = viewer_current_signatures[sig]

        user_profile.save()

        return HttpResponse(status=201)


class Delete(View):
    """View for deleting a user profile."""

    def get(self, request: HttpRequest):  # pragma: no cover
        """Display the page for deleting the user"""

        return render(request, 'profile_delete.html')

    def post(self, request: HttpRequest):
        """Delete the user"""

        user = request.user  # type: ignore

        logout(request)
        user.delete()
        messages.success(request, 'Your Account was successfully deleted.')

        return redirect('home')


@method_decorator(login_not_required, name='dispatch')
class AdminLoginView(View):
    """Single-admin password login view."""

    def get(self, request):
        from users.forms import AdminLoginForm
        return render(request, 'login.html', {'form': AdminLoginForm()})

    def post(self, request):
        from users.forms import AdminLoginForm
        form = AdminLoginForm(request.POST)
        if form.is_valid():
            password = form.cleaned_data['password']
            user = User.objects.filter(email=django_settings.ADMIN_EMAIL).first()
            if not user:
                user = User.objects.filter(is_superuser=True).order_by('id').first()
            if user and user.check_password(password):
                auth_login(request, user, backend='django.contrib.auth.backends.ModelBackend')
                return redirect(django_settings.LOGIN_REDIRECT_URL)
        return render(request, 'login.html', {'form': form, 'error': _('Invalid password')})
