from django.conf import settings
from django.contrib.auth.decorators import login_not_required
from django.http import HttpRequest, HttpResponse
from django.utils.decorators import method_decorator
from django.views import View


@method_decorator(login_not_required, name="dispatch")
class HealthView(View):
    """View for the health check endpoint."""

    def get(self, request: HttpRequest):
        return HttpResponse(status=200)


@method_decorator(login_not_required, name='dispatch')
class ServiceWorkerView(View):
    """Serve the service worker at root scope."""

    def get(self, request: HttpRequest):
        sw_path = settings.BASE_DIR / 'static' / 'js' / 'service-worker.js'
        with open(sw_path, 'r') as f:
            content = f.read()
        response = HttpResponse(content, content_type='text/javascript')
        response['Service-Worker-Allowed'] = '/'
        return response
