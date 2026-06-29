from core.views import HealthView, ServiceWorkerView
from django.contrib.auth.views import LogoutView
from django.urls import include, path
from pdf.views.pdf_views import redirect_to_overview
from users.views import AdminLoginView

urlpatterns = [
    path('login/', AdminLoginView.as_view(), name='login'),
    path('logout/', LogoutView.as_view(next_page='login'), name='logout'),
    path('admin/', include('admin.urls')),
    path('', redirect_to_overview, name='home'),
    path('profile/', include('users.urls')),
    path('pdf/', include('pdf.urls')),
    path('healthz', HealthView.as_view(), name='healthz'),
    path('service-worker.js', ServiceWorkerView.as_view(), name='service_worker'),
]
