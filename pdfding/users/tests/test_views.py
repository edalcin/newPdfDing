from datetime import datetime, timezone
from unittest.mock import patch

from django.contrib.auth.models import User
from django.test import Client, TestCase, override_settings
from django.urls import reverse
from pdf.models.pdf_models import Pdf
from pdf.models.tag_models import Tag
from users import forms
from users.models import Profile

from pdfding.pdf.services.workspace_services import create_workspace


class TestAuthRelated(TestCase):
    def test_login_required(self):
        response = self.client.get(reverse('pdf_overview'))

        self.assertRedirects(response, f'/login/?next={reverse("pdf_overview")}', status_code=302)

    def test_login(self):
        response = self.client.get(reverse('login'))

        self.assertEqual(response.status_code, 200)



class BaseProfileView(TestCase):
    username = 'user'
    password = '12345'

    def setUp(self):
        self.client = Client()
        self.user = User.objects.create_user(username=self.username, password=self.password, email='a@a.com')
        self.client.login(username=self.username, password=self.password)


class TestProfileSettingsViews(BaseProfileView):
    def test_settings(self):
        response = self.client.get(reverse('account_settings'))
        self.assertEqual(response.status_code, 200)


    def test_change_settings_get_no_htmx(self):
        response = self.client.get(reverse('profile-setting-change', kwargs={'field_name': 'email'}))

        # target_status_code=302 because the '/' will redirect to the pdf overview
        self.assertRedirects(response, '/', status_code=302, target_status_code=302)

    def test_change_settings_get_htmx(self):
        self.user.profile.dark_mode = 'Light'
        self.user.profile.theme_color = 'Green'
        self.user.profile.save()

        headers = {'HTTP_HX-Request': 'true'}

        field_names = [
            'pdf_inverted_mode',
            "pdf_keep_screen_awake",
            'custom_theme_color',
            'theme',
            'theme_color',
            'email',
            'show_progress_bars',
            'language',
        ]
        form_list = [
            forms.GenericUserFieldForm,
            forms.GenericUserFieldForm,
            forms.CustomThemeColorForm,
            forms.GenericUserFieldForm,
            forms.GenericUserFieldForm,
            forms.EmailForm,
            forms.GenericUserFieldForm,
            forms.GenericUserFieldForm,
        ]
        initial_dicts = [
            {'pdf_inverted_mode': 'Disabled'},
            {'pdf_keep_screen_awake': 'Disabled'},
            {'custom_theme_color': '#ffa385'},
            {'dark_mode': 'Light'},
            {'theme_color': 'Green'},
            {'email': 'a@a.com'},
            {'show_progress_bars': 'Enabled'},
            {'language': 'English'},
        ]

        for field_name, form, initial_dict in zip(field_names, form_list, initial_dicts):
            response = self.client.get(reverse('profile-setting-change', kwargs={'field_name': field_name}), **headers)

            self.assertIsInstance(response.context['form'], form)
            self.assertEqual(initial_dict, response.context['form'].initial)

    def test_change_settings_post_invalid_form(self):
        # follow=True is needed for getting the message
        response = self.client.post(
            reverse('profile-setting-change', kwargs={'field_name': 'custom_theme_color'}),
            data={"custom_theme_color": 'invalid'},
            follow=True,
        )
        message = list(response.context['messages'])[0]

        self.assertEqual(message.message, 'Only valid hex colors are allowed! E.g.: #ffa385.')
        self.assertEqual(message.tags, 'warning')

    def test_change_settings_email_post_email_exists(self):
        User.objects.create_user(username='other_user', password=self.password, email='a@b.com')
        # follow=True is needed for getting the message
        response = self.client.post(
            reverse('profile-setting-change', kwargs={'field_name': 'email'}), data={"email": 'a@b.com'}, follow=True
        )
        message = list(response.context['messages'])[0]

        self.assertEqual(message.message, 'a@b.com is already in use.')
        self.assertEqual(message.tags, 'warning')

    def test_change_settings_email_post_correct(self):
        self.client.post(reverse('profile-setting-change', kwargs={'field_name': 'email'}), data={'email': 'a@c.com'})

        # get the user and check if email was changed
        user = User.objects.get(username=self.username)
        self.assertEqual(user.email, 'a@c.com')

    def test_change_settings_custom_theme_color(self):
        self.client.post(
            reverse('profile-setting-change', kwargs={'field_name': 'custom_theme_color'}),
            data={'custom_theme_color': '#b5edff'},
        )

        # get the user and check if email was changed
        user = User.objects.get(username=self.username)
        self.assertEqual(user.profile.custom_theme_color, '#b5edff')
        self.assertEqual(user.profile.custom_theme_color_secondary, '#91becc')

    def test_change_settings_dark_mode_post_correct(self):
        self.user.profile.dark_mode = 'Light'
        self.user.profile.save()

        self.assertEqual(self.user.profile.dark_mode, 'Light')
        self.client.post(
            reverse('profile-setting-change', kwargs={'field_name': 'theme'}),
            data={'dark_mode': 'Dark'},
        )

        # get the user and check if dark mode was changed
        user = User.objects.get(username=self.username)
        self.assertEqual(user.profile.dark_mode, 'Dark')

    def test_change_settings_theme_color_post_correct(self):
        self.user.profile.theme_color = 'Green'
        self.user.profile.save()

        self.assertEqual(self.user.profile.theme_color, 'Green')
        self.client.post(
            reverse('profile-setting-change', kwargs={'field_name': 'theme_color'}),
            data={'theme_color': 'Blue'},
        )

        # get the user and check if dark mode was changed
        user = User.objects.get(username=self.username)
        self.assertEqual(user.profile.theme_color, 'Blue')

    def test_change_settings_normal_post_correct(self):
        for field_name, val_before, val_after in zip(
            ['pdf_inverted_mode', 'pdf_keep_screen_awake', 'show_progress_bars', 'language'],
            ['Disabled', 'Disabled', 'Enabled', 'English'],
            ['Enabled', 'Enabled', 'Disabled', 'Auto'],
        ):
            self.assertEqual(getattr(self.user.profile, field_name), val_before)
            self.client.post(
                reverse('profile-setting-change', kwargs={'field_name': field_name}), data={field_name: val_after}
            )

            user = User.objects.get(username=self.username)
            self.assertEqual(getattr(user.profile, field_name), val_after)

    def test_change_sorting_post_no_htmx(self):
        response = self.client.post(
            reverse('change_sorting', kwargs={'sorting_category': 'pdf_sorting', 'sorting': 'Oldest'})
        )

        self.assertRedirects(response, reverse('account_settings'), status_code=302)

    def test_change_sorting_post_pdf(self):
        self.assertEqual(self.user.profile.pdf_sorting, Profile.PdfSortingChoice.NEWEST)

        headers = {'HTTP_HX-Request': 'true'}
        self.client.post(
            reverse('change_sorting', kwargs={'sorting_category': 'pdf_sorting', 'sorting': 'oldest'}), **headers
        )
        changed_user = User.objects.get(id=self.user.id)
        self.assertEqual(changed_user.profile.pdf_sorting, Profile.PdfSortingChoice.OLDEST)

    def test_change_tree_mode_post_no_htmx(self):
        response = self.client.post(reverse('change_tree_mode'))

        self.assertRedirects(response, reverse('account_settings'), status_code=302)

    def test_change_layout_post(self):
        self.assertEqual(self.user.profile.layout, Profile.LayoutChoice.COMPACT)

        headers = {'HTTP_HX-Request': 'true'}
        self.client.post(reverse('change_layout', kwargs={'layout': 'grid'}), **headers)
        changed_user = User.objects.get(id=self.user.id)
        self.assertEqual(changed_user.profile.layout, Profile.LayoutChoice.GRID)

    def test_change_layout_post_no_htmx(self):
        response = self.client.post(reverse('change_layout', kwargs={'layout': 'grid'}))

        self.assertRedirects(response, reverse('account_settings'), status_code=302)

    def test_change_tree_mode_post(self):
        self.assertTrue(self.user.profile.tag_tree_mode)

        headers = {'HTTP_HX-Request': 'true'}

        self.client.post(reverse('change_tree_mode'), **headers)
        changed_user = User.objects.get(id=self.user.id)
        self.assertFalse(changed_user.profile.tag_tree_mode)

        self.client.post(reverse('change_tree_mode'), **headers)
        changed_user = User.objects.get(id=self.user.id)
        self.assertTrue(changed_user.profile.tag_tree_mode)

    def test_open_collapse_tags_post_no_htmx(self):
        response = self.client.post(reverse('open_collapse_tags'))

        self.assertRedirects(response, reverse('account_settings'), status_code=302)

    def test_open_collapse_tags_post(self):
        self.assertFalse(self.user.profile.tags_open)

        headers = {'HTTP_HX-Request': 'true'}

        self.client.post(reverse('open_collapse_tags'), **headers)
        changed_user = User.objects.get(id=self.user.id)
        self.assertTrue(changed_user.profile.tags_open)

        self.client.post(reverse('open_collapse_tags'), **headers)
        changed_user = User.objects.get(id=self.user.id)
        self.assertFalse(changed_user.profile.tags_open)

    def test_update_last_time_nagged_no_htmx(self):
        response = self.client.post(reverse('update_last_time_nagged'))

        self.assertRedirects(response, reverse('pdf_overview'), status_code=302)

    def test_a_update_last_time_nagged(self):
        self.assertTrue((datetime.now(timezone.utc) - self.user.profile.last_time_nagged).total_seconds() > 1000)

        headers = {'HTTP_HX-Request': 'true'}

        self.client.post(reverse('update_last_time_nagged'), **headers)
        changed_user = User.objects.get(id=self.user.id)
        self.assertTrue((datetime.now(timezone.utc) - changed_user.profile.last_time_nagged).total_seconds() < 0.1)

    def test_change_sorting_post_shared_pdf(self):
        self.assertEqual(self.user.profile.shared_pdf_sorting, Profile.SharedPdfSortingChoice.NEWEST)

        headers = {'HTTP_HX-Request': 'true'}
        self.client.post(
            reverse('change_sorting', kwargs={'sorting_category': 'shared_pdf_sorting', 'sorting': 'oldest'}), **headers
        )
        changed_user = User.objects.get(id=self.user.id)
        self.assertEqual(changed_user.profile.shared_pdf_sorting, Profile.SharedPdfSortingChoice.OLDEST)

    def test_change_sorting_post_user(self):
        self.assertEqual(self.user.profile.user_sorting, Profile.UserSortingChoice.NEWEST)

        headers = {'HTTP_HX-Request': 'true'}
        self.client.post(
            reverse('change_sorting', kwargs={'sorting_category': 'user_sorting', 'sorting': 'oldest'}), **headers
        )
        changed_user = User.objects.get(id=self.user.id)
        self.assertEqual(changed_user.profile.user_sorting, Profile.UserSortingChoice.OLDEST)

    def test_change_sorting_post_annotation(self):
        self.assertEqual(self.user.profile.annotation_sorting, Profile.AnnotationsSortingChoice.NEWEST)

        headers = {'HTTP_HX-Request': 'true'}
        self.client.post(
            reverse('change_sorting', kwargs={'sorting_category': 'annotation_sorting', 'sorting': 'oldest'}), **headers
        )
        changed_user = User.objects.get(id=self.user.id)
        self.assertEqual(changed_user.profile.annotation_sorting, Profile.AnnotationsSortingChoice.OLDEST)


    def test_change_collection_no_htmx(self):
        response = self.client.post(reverse('change_collection', kwargs={'collection_id': '1'}))

        self.assertRedirects(response, reverse('pdf_overview'), status_code=302)

    def test_change_collection_post_all(self):
        headers = {'HTTP_HX-Request': 'true'}
        self.client.post(reverse('change_collection', kwargs={'collection_id': 'all'}), **headers)

        changed_user = User.objects.get(id=self.user.id)
        assert changed_user.profile.current_collection_id == 'all'

    @patch('users.views.check_if_collection_part_of_workspace', return_value=True)
    def test_change_collection_post_check_true(self, mock_check):
        headers = {'HTTP_HX-Request': 'true'}
        self.client.post(reverse('change_collection', kwargs={'collection_id': '3'}), **headers)

        changed_user = User.objects.get(id=self.user.id)
        assert changed_user.profile.current_collection_id == '3'

    @patch('users.views.check_if_collection_part_of_workspace', return_value=False)
    def test_change_collection_post_no_access(self, mock_check):
        headers = {'HTTP_HX-Request': 'true'}
        response = self.client.post(reverse('change_collection', kwargs={'collection_id': '4'}), **headers)

        self.assertEqual(response.status_code, 404)


class TestProfileOtherViews(BaseProfileView):
    def test_delete_post(self):
        # in this test we test that the user is successfully deleted
        # we also test that the associated profile, pdfs, and tags are also deleted
        pdf = Pdf.objects.create(name='pdf_1', collection=self.user.profile.current_collection)
        tags = [Tag.objects.create(name='tag', workspace=self.user.profile.current_workspace)]
        pdf.tags.set(tags)

        for model_class in [Profile, Pdf, Tag]:
            # assert there is a profile, pdf and tag
            self.assertEqual(model_class.objects.all().count(), 1)

        # follow=True is needed for getting the message
        response = self.client.post(reverse('profile-delete'), follow=True)
        message = list(response.context['messages'])[0]

        self.assertFalse(User.objects.filter(username=self.username).exists())
        self.assertEqual(message.message, 'Your Account was successfully deleted.')
        self.assertEqual(message.tags, 'success')
        for model_class in [Profile, Pdf, Tag]:
            # assert there is no profile, pdf and tag
            self.assertEqual(model_class.objects.all().count(), 0)

    def test_get_signatures(self):
        signatures = {'1': 'signature_1', '2': 'signature_2'}

        profile = self.user.profile
        profile.signatures = signatures
        profile.save()

        response = self.client.get(reverse('signatures'))

        self.assertEqual(response.status_code, 200)  # type: ignore
        self.assertEqual(response.json(), signatures)  # type: ignore

    def test_post_signatures(self):
        current_signatures = '{"old_2": "old2", "new": "something_new"}'
        previous_signatures = '{"old_1": "old1", "old_2": "old2", "old_3": "old3"}'
        server_signatures = {'old_1': 'old1', 'old_2': 'old2'}

        profile = self.user.profile
        profile.signatures = server_signatures
        profile.save()

        response = self.client.post(
            reverse('signatures'),
            data={'current_signatures': current_signatures, 'previous_signatures': previous_signatures},
        )

        changed_user = User.objects.get(id=self.user.id)

        self.assertEqual(response.status_code, 201)  # type: ignore
        self.assertEqual(changed_user.profile.signatures, {'old_2': 'old2', 'new': 'something_new'})


