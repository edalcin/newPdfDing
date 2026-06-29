from django.conf import settings
from django.core.files import File
from users.models import Profile


def convert_hex_to_rgb(hex_color: str) -> tuple[int, ...]:
    """Converts a hex color representation to RGB representation"""

    hex_color = hex_color.replace('#', '')

    return tuple(int(hex_color[i : i + 2], 16) for i in (0, 2, 4))  # noqa


def convert_rgb_to_hex(r: int, g: int, b: int) -> str:
    """Converts RGB color representation to a hex representation"""

    hex_color = ('{:02X}' * 3).format(r, g, b)

    return f'#{str.lower(hex_color)}'


def darken_color(red: int, green: int, blue: int, percentage: float) -> tuple[int, ...]:
    """Darkening a RGB color by the specified percentage"""

    correction_factor = 1 - percentage

    return tuple(round(val * correction_factor) for val in (red, green, blue))


def get_secondary_color(custom_color: str) -> str:
    """Get the secondary color of a custom color."""

    rgb_color = convert_hex_to_rgb(str(custom_color))

    secondary_color = darken_color(*rgb_color, percentage=0.2)

    return convert_rgb_to_hex(*secondary_color)


def get_viewer_theme_and_color(user_profile: Profile | None = None) -> tuple[str, str]:
    theme_color_dict = {
        'Green': '74 222 128',
        'Blue': '71 147 204',
        'Gray': '151 170 189',
        'Red': '248 113 113',
        'Pink': '218 123 147',
        'Orange': '255 203 133',
        'Brown': '76 37 24',
    }

    if user_profile:
        rgb_as_str = [str(val) for val in convert_hex_to_rgb(user_profile.custom_theme_color)]
        custom_color_rgb_str = ' '.join(rgb_as_str)
        theme_color_dict['Custom'] = custom_color_rgb_str

        theme_color = user_profile.theme_color

        if user_profile.pdf_inverted_mode == 'Enabled':
            theme = 'inverted'
        else:
            theme = user_profile.dark_mode_str
    else:
        theme_color = settings.DEFAULT_THEME_COLOR
        theme = settings.DEFAULT_THEME

    return theme, theme_color_dict[theme_color]


def get_demo_pdf():
    """Get the PDF file used for testing."""
    file_path = settings.BASE_DIR / 'users' / 'demo_data' / 'demo.pdf'
    return File(file=open(file_path, 'rb'), name=file_path.name)

def get_example_notes():  # pragma: no cover
    """
    Get example notes for demo user
    """

    notes = '''
## **`Example Note`**

In the notes you can add further information about your PDF. Notes support markdown, so it is possible to use `inline code` or [links](https://github.com/mrmn2/PdfDing). You can also use lists:

* here is an example element
    * nested lists are also possible
* here is another one

Of course you can also use code blocks

```
# example code
def example_code():
    print('hi')
```
or block quotes
> this is a block quote
>> with a nested block quote
'''  # noqa: E501

    return notes
