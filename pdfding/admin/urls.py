from admin import views
from django.urls import path

urlpatterns = [
    path('info', views.Information.as_view(), name='instance_info'),
]
