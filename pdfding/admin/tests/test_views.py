from django.contrib.auth.models import User
from django.test import TestCase
from django.urls import reverse


class TestAdminViews(TestCase):
    def setUp(self):
        self.user = User.objects.create_user(username='admin', password='password', email='a@a.com')
        self.user.is_staff = True
        self.user.is_superuser = True
        self.user.save()

        self.client.login(username='admin', password='password')

    def test_get_information(self):
        response = self.client.get(reverse('instance_info'))

        self.assertEqual(response.status_code, 200)


class TestAdminTagViews(TestCase):
    def setUp(self):
        self.user = User.objects.create_user(username='admin', password='password', email='a@a.com')
        self.user.is_staff = True
        self.user.is_superuser = True
        self.user.save()
        self.client.login(username='admin', password='password')

    def _workspace(self):
        return self.user.profile.current_workspace

    def test_overview_get(self):
        response = self.client.get(reverse('admin_tag_overview'))
        self.assertEqual(response.status_code, 200)
        self.assertTemplateUsed(response, 'tag_management.html')

    def test_non_admin_404(self):
        other = User.objects.create_user(username='plain', password='password', email='b@b.com')
        self.client.login(username='plain', password='password')
        response = self.client.get(reverse('admin_tag_overview'))
        self.assertEqual(response.status_code, 404)

    def test_create_tag(self):
        self.client.post(reverse('admin_tag_create'), {'name': 'foo'})
        self.assertTrue(self._workspace().tag_set.filter(name='foo').exists())

    def test_create_tag_duplicate(self):
        from pdf.models.tag_models import Tag
        Tag.objects.create(name='foo', workspace=self._workspace())
        response = self.client.post(reverse('admin_tag_create'), {'name': 'foo'}, follow=True)
        message = list(response.context['messages'])[0]
        self.assertEqual(message.tags, 'warning')
        self.assertEqual(self._workspace().tag_set.filter(name='foo').count(), 1)

    def test_create_tag_invalid(self):
        response = self.client.post(reverse('admin_tag_create'), {'name': 'has space'}, follow=True)
        message = list(response.context['messages'])[0]
        self.assertEqual(message.tags, 'warning')
        self.assertFalse(self._workspace().tag_set.filter(name='has space').exists())

    def test_rename_tag(self):
        from pdf.models.tag_models import Tag
        tag = Tag.objects.create(name='old', workspace=self._workspace())
        self.client.post(reverse('admin_tag_rename'), {'tag_id': tag.id, 'name': 'new'})
        tag.refresh_from_db()
        self.assertEqual(tag.name, 'new')

    def test_rename_tag_collision(self):
        from pdf.models.tag_models import Tag
        a = Tag.objects.create(name='alpha', workspace=self._workspace())
        Tag.objects.create(name='beta', workspace=self._workspace())
        response = self.client.post(reverse('admin_tag_rename'), {'tag_id': a.id, 'name': 'beta'}, follow=True)
        message = list(response.context['messages'])[0]
        self.assertEqual(message.tags, 'warning')
        a.refresh_from_db()
        self.assertEqual(a.name, 'alpha')

    def test_delete_tag(self):
        from pdf.models.tag_models import Tag
        tag = Tag.objects.create(name='todel', workspace=self._workspace())
        self.client.post(reverse('admin_tag_delete'), {'tag_id': tag.id})
        self.assertFalse(Tag.objects.filter(id=tag.id).exists())

    def test_substitute_tag(self):
        from pdf.models.pdf_models import Pdf
        from pdf.models.tag_models import Tag
        pdf = Pdf.objects.create(collection=self.user.profile.current_collection, name='pdf_sub')
        src = Tag.objects.create(name='src', workspace=self._workspace())
        dst = Tag.objects.create(name='dst', workspace=self._workspace())
        pdf.tags.add(src)
        self.client.post(reverse('admin_tag_substitute'), {'source_id': src.id, 'target_id': dst.id})
        pdf.refresh_from_db()
        self.assertFalse(Tag.objects.filter(id=src.id).exists())
        self.assertIn(dst, pdf.tags.all())

    def test_substitute_same(self):
        from pdf.models.tag_models import Tag
        tag = Tag.objects.create(name='same', workspace=self._workspace())
        response = self.client.post(reverse('admin_tag_substitute'), {'source_id': tag.id, 'target_id': tag.id}, follow=True)
        message = list(response.context['messages'])[0]
        self.assertEqual(message.tags, 'warning')
        self.assertTrue(Tag.objects.filter(id=tag.id).exists())


class TestAdminSharedPdfViews(TestCase):
    def setUp(self):
        self.user = User.objects.create_user(username='admin', password='password', email='a@a.com')
        self.user.is_staff = True
        self.user.is_superuser = True
        self.user.save()
        self.client.login(username='admin', password='password')

    def _workspace(self):
        return self.user.profile.current_workspace

    def _create_pdf(self):
        from django.core.files.uploadedfile import SimpleUploadedFile
        from pdf.models.pdf_models import Pdf
        f = SimpleUploadedFile('x.pdf', b'%PDF-1.4')
        return Pdf.objects.create(collection=self.user.profile.current_collection, name='x', file=f)

    def test_overview_get(self):
        response = self.client.get(reverse('admin_shared_overview'))
        self.assertEqual(response.status_code, 200)
        self.assertTemplateUsed(response, 'shared_pdf_management.html')

    def test_non_admin_404(self):
        other = User.objects.create_user(username='plain', password='password', email='b@b.com')
        self.client.login(username='plain', password='password')
        response = self.client.get(reverse('admin_shared_overview'))
        self.assertEqual(response.status_code, 404)

    def test_revoke(self):
        from pdf.models.shared_models import SharedPdf
        pdf = self._create_pdf()
        share = SharedPdf.objects.create(pdf=pdf)
        self.client.post(reverse('admin_share_revoke'), {'share_id': share.id})
        self.assertFalse(SharedPdf.objects.filter(id=share.id).exists())

    def test_revoke_not_found(self):
        from uuid import uuid4
        response = self.client.post(reverse('admin_share_revoke'), {'share_id': uuid4()}, follow=True)
        messages = list(response.context['messages'])
        self.assertEqual(messages[0].tags, 'warning')
