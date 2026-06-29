from datetime import datetime, timedelta, timezone

from django.contrib.auth.models import User
from django.test import override_settings
from django.urls import reverse
from helpers import PdfDingE2ENoLoginTestCase, PdfDingE2ETestCase
from playwright.sync_api import expect, sync_playwright
from users.models import NAGGING_INTERVAL_WEEKS


class UsersE2ETestCase(PdfDingE2ETestCase):
    def test_settings_change_theme(self):
        self.user.profile.dark_mode = 'Light'
        self.user.profile.save()

        with sync_playwright() as p:
            self.open(reverse('ui_settings'), p)

            # test that light theme is used
            expect(self.page.locator('html')).to_have_attribute('class', 'light')
            expect(self.page.locator("#theme")).to_contain_text("Light")
            expect(self.page.locator('body')).to_have_css('background-color', 'oklch(0.984 0.003 247.858)')

            # change to dark mode
            self.page.locator("#theme_edit").click()
            # check that selected option is correct
            expect(self.page.locator("#id_dark_mode")).to_have_value("Light")
            self.page.locator("#id_dark_mode").select_option("Dark")
            self.page.get_by_role("button", name="Submit").click()

            # check that theme was changed to dark
            expect(self.page.locator('html')).to_have_attribute('class', 'dark')
            expect(self.page.locator("#theme")).to_contain_text("Dark")
            expect(self.page.locator('body')).to_have_css('background-color', 'oklch(0.208 0.042 265.755)')

            # trigger dropdown again
            self.page.locator("#theme_edit").click()
            # check that selected option is correct
            expect(self.page.locator("#id_dark_mode")).to_have_value("Dark")

    def test_settings_change_theme_color(self):
        self.user.profile.theme_color = 'Green'
        self.user.profile.custom_theme_color = '#ffa385'
        self.user.profile.save()

        with sync_playwright() as p:
            self.open(reverse('ui_settings'), p)

            # test that light theme is used
            expect(self.page.locator('html')).to_have_attribute('data-theme', 'Green')
            expect(self.page.locator("#theme_color")).to_contain_text("Green")
            expect(self.page.locator('#logo_div')).to_have_css('background-color', 'rgb(74, 222, 128)')

            # change to dark mode
            self.page.locator("#theme_color_edit").click()
            # check that selected option is correct
            expect(self.page.locator("#id_theme_color")).to_have_value("Green")
            self.page.locator("#id_theme_color").select_option("Custom")
            self.page.get_by_role("button", name="Submit").click()

            # check that theme was changed to dark
            expect(self.page.locator('html')).to_have_attribute('data-theme', 'Custom')
            expect(self.page.locator("#theme_color")).to_contain_text("Custom")
            expect(self.page.locator('#logo_div')).to_have_css('background-color', 'rgb(255, 163, 133)')

            # trigger dropdown again
            self.page.locator("#theme_color_edit").click()
            # check that selected option is correct
            expect(self.page.locator("#id_theme_color")).to_have_value("Custom")

    def test_settings_email_change(self):
        with sync_playwright() as p:
            self.open(reverse('account_settings'), p)

            # check email address before changing
            expect(self.page.locator('#email_address')).to_contain_text('a@a.com')
            expect(self.page.locator('body')).to_contain_text('Verified')

            # change email address
            self.page.locator('#email_edit').click()
            self.page.locator('#id_email').click()
            self.page.locator('#id_email').press('ControlOrMeta+a')
            self.page.locator('#id_email').fill('a@b.com')
            self.page.get_by_role('button', name='Submit').click()

            # check email address after changing
            expect(self.page.locator('#email_address')).to_contain_text('a@b.com')
            expect(self.page.locator('body')).to_contain_text('Not verified')

    def test_settings_change_language(self):
        with sync_playwright() as p:
            self.open(reverse('account_settings'), p)

            expect(self.page.locator("#language")).to_contain_text("English")

            self.page.locator("#language_edit").click()
            self.page.locator("#id_language").select_option("Auto")
            self.page.get_by_role("button", name="Submit").click()

            expect(self.page.locator("#language")).to_contain_text("Auto")

    @override_settings(VERSION='not_dev')
    def test_settings_change_language_not_visible_prod(self):
        with sync_playwright() as p:
            self.open(reverse('account_settings'), p)

            expect(self.page.locator("#language")).not_to_be_visible()

    def test_settings_change_custom_color(self):
        with sync_playwright() as p:
            self.open(reverse('ui_settings'), p)

            # check custom color before changing
            expect(self.page.locator("#custom_theme_color")).to_contain_text("#ffa385")

            # change custom color
            self.page.locator("#custom_theme_color_edit").click()
            self.page.locator("#id_custom_theme_color").fill("#95c2d6")
            self.page.get_by_role("button", name="Submit").click()

            # check custom color after changing
            expect(self.page.locator("#custom_theme_color")).to_contain_text("95c2d6")

    def test_settings_change_inverted_pdf(self):
        with sync_playwright() as p:
            self.open(reverse('viewer_settings'), p)

            # check inverted color mode before changing
            expect(self.page.locator("#pdf_inverted_mode")).to_contain_text("Disabled")

            # change inverted color mode
            self.page.locator("#pdf_inverted_mode_edit").click()
            self.page.locator("#id_pdf_inverted_mode").select_option("Enabled")
            self.page.get_by_role("button", name="Submit").click()

            # check inverted color mode after changing
            expect(self.page.locator("#pdf_inverted_mode")).to_contain_text("Enabled")

    def test_settings_change_keep_awake(self):
        with sync_playwright() as p:
            self.open(reverse('viewer_settings'), p)

            # check inverted color mode before changing
            expect(self.page.locator("#pdf_keep_screen_awake")).to_contain_text("Disabled")

            # change inverted color mode
            self.page.locator("#pdf_keep_screen_awake_edit").click()
            self.page.locator("#id_pdf_keep_screen_awake").select_option("Enabled")
            self.page.get_by_role("button", name="Submit").click()

            # check inverted color mode after changing
            expect(self.page.locator("#pdf_keep_screen_awake")).to_contain_text("Enabled")

    def test_settings_change_show_progress_bars(self):
        with sync_playwright() as p:
            self.open(reverse('ui_settings'), p)

            expect(self.page.locator("#show_progress_bars")).to_contain_text("Enabled")

            self.page.locator("#show_progress_bars_edit").click()
            self.page.locator("#id_show_progress_bars").select_option("Disabled")
            self.page.get_by_role("button", name="Submit").click()

            expect(self.page.locator("#show_progress_bars")).to_contain_text("Disabled")

    def test_settings_delete(self):
        with sync_playwright() as p:
            self.open(reverse('danger_settings'), p)

            # we just check if delete button is displayed, rest is covered by unit test
            self.page.get_by_role('link', name='Delete').click()
            expect(self.page.get_by_role('button')).to_contain_text('Yes, I want to delete my account')

    def test_settings_edit_cancel_account_settings(self):
        with sync_playwright() as p:
            self.open(reverse('account_settings'), p)

            for name in ['#email_edit']:
                self.page.locator(name).click()
                expect(self.page.locator(name)).to_contain_text('Cancel')
                self.page.get_by_text("Cancel").click()
                expect(self.page.locator(name)).to_contain_text('Edit')

    def test_settings_edit_cancel_ui_settings(self):
        with sync_playwright() as p:
            self.open(reverse('ui_settings'), p)

            for name in ['#theme_edit', '#theme_color_edit', '#custom_theme_color_edit', '#show_progress_bars_edit']:
                self.page.locator(name).click()
                expect(self.page.locator(name)).to_contain_text('Cancel')
                self.page.get_by_text("Cancel").click()
                expect(self.page.locator(name)).to_contain_text('Edit')

    def test_settings_edit_cancel_viewer_settings(self):
        with sync_playwright() as p:
            self.open(reverse('viewer_settings'), p)

            for name in ['#pdf_inverted_mode_edit', '#pdf_keep_screen_awake_edit']:
                self.page.locator(name).click()
                expect(self.page.locator(name)).to_contain_text('Cancel')
                self.page.get_by_text("Cancel").click()
                expect(self.page.locator(name)).to_contain_text('Edit')

    def test_header_dropdown(self):
        with sync_playwright() as p:
            self.open(reverse('pdf_overview'), p)
            self.page.locator("#open-user-dropdown > div:nth-child(3)").click()
            expect(self.page.get_by_role("banner")).to_contain_text("a@a.com")
            expect(self.page.get_by_role("banner")).to_contain_text(f"User ID: {self.user.id}")

    def test_header_non_admin(self):
        with sync_playwright() as p:
            self.open(reverse('pdf_overview'), p)
            expect(self.page.get_by_role("banner")).not_to_contain_text("Admin")

    def test_header_admin(self):
        self.user.is_staff = True
        self.user.is_superuser = True

        self.user.save()

        with sync_playwright() as p:
            self.open(reverse('pdf_overview'), p)
            expect(self.page.get_by_role("banner")).to_contain_text("Admin")

    def test_nagging_banner_needs_nagging(self):
        self.user.profile.last_time_nagged = datetime.now(tz=timezone.utc) - timedelta(weeks=NAGGING_INTERVAL_WEEKS + 1)
        self.user.profile.save()

        self.assertTrue(self.user.profile.needs_nagging)

        with sync_playwright() as p:
            self.open(reverse('pdf_overview'), p)
            expect(self.page.locator("#nagging_banner")).to_be_visible()

            self.page.locator("#close_nagging_banner").click()
            expect(self.page.locator("#nagging_banner")).not_to_be_visible()

        changed_user = User.objects.get(id=self.user.id)
        self.assertFalse(changed_user.profile.needs_nagging)

    def test_nagging_banner_needs_no_nagging(self):
        self.user.profile.last_time_nagged = datetime.now(tz=timezone.utc) - timedelta(weeks=NAGGING_INTERVAL_WEEKS - 1)
        self.user.profile.save()

        with sync_playwright() as p:
            self.open(reverse('pdf_overview'), p)
            expect(self.page.locator("#nagging_banner")).not_to_be_visible()


class UsersLoginE2ETestCase(PdfDingE2ENoLoginTestCase):


    @override_settings(DEFAULT_THEME='dark', DEFAULT_THEME_COLOR='Blue')
    def test_default_theme(self):
        with sync_playwright() as p:
            self.open(reverse('home'), p)

            # test that light theme is used
            expect(self.page.locator('html')).to_have_attribute('class', 'dark')
            expect(self.page.locator('html')).to_have_attribute('data-theme', 'Blue')
            expect(self.page.locator('body')).to_have_css('background-color', 'oklch(0.208 0.042 265.755)')
            expect(self.page.locator('#logo_div')).to_have_css('background-color', 'rgb(71, 147, 204)')

