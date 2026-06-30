from admin import views
from django.urls import path

urlpatterns = [
    path('info', views.Information.as_view(), name='instance_info'),
    path('tags', views.TagOverview.as_view(), name='admin_tag_overview'),
    path('tags/create', views.CreateTag.as_view(), name='admin_tag_create'),
    path('tags/rename', views.RenameTag.as_view(), name='admin_tag_rename'),
    path('tags/delete', views.DeleteTag.as_view(), name='admin_tag_delete'),
    path('tags/substitute', views.SubstituteTag.as_view(), name='admin_tag_substitute'),
]
