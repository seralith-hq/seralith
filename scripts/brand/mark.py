"""Construction of the Sigil symbol.

The symbol is a pointy-top hexagon outline and a single folded band (the S) that
leaves the left wall and re-enters the right wall. All edges run at 30 degrees or
vertical, and every band and every gap has the same width t, which fixes the
proportions: t = R cos(30) / 4.5 for an outer radius R.
"""

import math

from shapely.geometry import LineString, Polygon
from shapely.ops import unary_union

COS30 = math.cos(math.radians(30))
SIN30 = 0.5


def hexagon(radius):
    return Polygon([
        (radius * math.cos(math.radians(a)), radius * math.sin(math.radians(a)))
        for a in (-90, -30, 30, 90, 150, 210)
    ])


def symbol(radius=100.0):
    t = radius * COS30 / 4.5
    a = 2 * t / COS30
    outer = hexagon(radius)
    ring = outer.difference(hexagon(radius - t / COS30))
    reach = radius / COS30
    path = LineString([
        (-reach * COS30, a - reach * SIN30),
        (0, a),
        (a * COS30, a * SIN30),
        (-a * COS30, -a * SIN30),
        (0, -a),
        (reach * COS30, -a + reach * SIN30),
    ])
    band = path.buffer(t / 2, cap_style="flat", join_style="mitre", mitre_limit=1.5)
    return unary_union([ring, band]).intersection(outer).buffer(0), t
