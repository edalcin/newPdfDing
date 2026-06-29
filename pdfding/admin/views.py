from django.conf import settings
from django.http import Http404
from django.shortcuts import render
from django.views import View


class Information(View):  # pragma: no cover
    """View for getting instance information"""

    def test_func(self, request):
        if not (request.user.is_superuser and request.user.is_staff):
            raise Http404

    def get(self, request):
        self.test_func(request)
        try:
            version = settings.VERSION
        except AttributeError:
            version = 'unknown'

        context = {'version': version}
        return render(request, 'information.html', context=context)
