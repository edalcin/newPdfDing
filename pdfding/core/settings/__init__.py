# Use dev settings as default, use production if dev settings do not exist
try:
    from .dev import *  # noqa: F401 F403
except ModuleNotFoundError:  # pragma: no cover
    from .prod import *  # noqa: F401 F403
