"""Shared geometry helpers for generating Sigil brand assets."""

from fontTools.pens.basePen import BasePen
from fontTools.ttLib import TTFont
from shapely.geometry import MultiPolygon, Polygon
from shapely.ops import unary_union


class FlattenPen(BasePen):
    def __init__(self, glyph_set, steps=16):
        super().__init__(glyph_set)
        self.rings, self.current, self.steps = [], [], steps

    def _moveTo(self, pt):
        self.current = [pt]

    def _lineTo(self, pt):
        self.current.append(pt)

    def _curveToOne(self, p1, p2, p3):
        p0 = self.current[-1]
        for i in range(1, self.steps + 1):
            t = i / self.steps
            u = 1 - t
            self.current.append((
                u**3 * p0[0] + 3 * u * u * t * p1[0] + 3 * u * t * t * p2[0] + t**3 * p3[0],
                u**3 * p0[1] + 3 * u * u * t * p1[1] + 3 * u * t * t * p2[1] + t**3 * p3[1],
            ))

    def _qCurveToOne(self, p1, p2):
        p0 = self.current[-1]
        for i in range(1, self.steps + 1):
            t = i / self.steps
            u = 1 - t
            self.current.append((
                u * u * p0[0] + 2 * u * t * p1[0] + t * t * p2[0],
                u * u * p0[1] + 2 * u * t * p1[1] + t * t * p2[1],
            ))

    def _closePath(self):
        if len(self.current) >= 3:
            self.rings.append(self.current)
        self.current = []

    _endPath = _closePath


def text_geometry(font_path, text, tracking=0.0):
    """Returns the outline of text in font units, y pointing down, baseline at y=0."""
    font = TTFont(font_path)
    glyph_set = font.getGlyphSet()
    cmap = font.getBestCmap()
    shapes, x = [], 0.0
    for ch in text:
        name = cmap[ord(ch)]
        pen = FlattenPen(glyph_set)
        glyph_set[name].draw(pen)
        glyph = None
        for ring in sorted((Polygon(r).buffer(0) for r in pen.rings), key=lambda p: -p.area):
            glyph = ring if glyph is None else glyph.symmetric_difference(ring)
        if glyph is not None:
            shapes.append(_transform(glyph, x))
        x += glyph_set[name].width + tracking
    return unary_union(shapes), font["head"].unitsPerEm


def _transform(geom, dx):
    from shapely.affinity import affine_transform
    return affine_transform(geom, [1, 0, 0, -1, dx, 0])


def to_path(geom, precision=2):
    """Serializes polygons to SVG path data."""
    polys = list(geom.geoms) if isinstance(geom, MultiPolygon) else [geom]
    parts = []
    for poly in polys:
        for ring in [poly.exterior, *poly.interiors]:
            coords = list(ring.coords)[:-1]
            pts = " ".join(f"{x:.{precision}f} {y:.{precision}f}" for x, y in coords)
            parts.append(f"M{pts}Z")
    return "".join(parts)
