from django.conf import settings
from django.contrib.auth.models import User
from django.http import Http404
from django.shortcuts import render
from django.views import View
from pdf.models.pdf_models import Pdf


class Information(View):  # pragma: no cover
    """View for getting instance information"""

    def test_func(self, request):
        if not (request.user.is_superuser and request.user.is_staff):
            raise Http404

    def get(self, request):
        self.test_func(request)
        context = {
            'current_version': getattr(settings, 'VERSION', 'unknown'),
            'number_of_users': User.objects.count(),
            'number_of_pdfs': Pdf.objects.count(),
        }
        return render(request, 'information.html', context=context)
