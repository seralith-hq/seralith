"""Construction of the Seralith symbol.

The symbol is "the locked stone": a standing stone with a peaked top, split by a
vertical seam, hollowed into a lock chamber at its center, and held together by
a single bolt driven straight through. Every measurement is a multiple of the
seam width u, and every slope runs at 1:2.
"""

from shapely.affinity import scale
from shapely.geometry import Polygon, box

HALF_WIDTH = 5.5
SHOULDER = 9.5
PEAK = SHOULDER + HALF_WIDTH / 2
CHAMBER_HALF = 2.0
CHAMBER_BOTTOM, CHAMBER_TOP = 2.0, 8.0
BOLT_BOTTOM, BOLT_TOP = 4.25, 5.75
MONO_GAP = 0.3


def _flip(geom):
    return scale(geom, 1, -1, origin=(0, 0))


def symbol():
    """Returns (stone, bolt, stone_mono) in seam units, y pointing down.

    stone_mono leaves a hairline gap around the bolt so the symbol still reads
    as two parts when printed in a single color.
    """
    outer = Polygon([
        (-HALF_WIDTH, 0), (HALF_WIDTH, 0), (HALF_WIDTH, SHOULDER),
        (0, PEAK), (-HALF_WIDTH, SHOULDER),
    ])
    seam = box(-0.5, -1, 0.5, PEAK + 1)
    step = (CHAMBER_HALF - 0.5) / 2
    chamber = Polygon([
        (-CHAMBER_HALF, CHAMBER_BOTTOM), (-0.5, CHAMBER_BOTTOM - step),
        (0.5, CHAMBER_BOTTOM - step), (CHAMBER_HALF, CHAMBER_BOTTOM),
        (CHAMBER_HALF, CHAMBER_TOP), (0.5, CHAMBER_TOP + step),
        (-0.5, CHAMBER_TOP + step), (-CHAMBER_HALF, CHAMBER_TOP),
    ])
    bolt = box(-HALF_WIDTH, BOLT_BOTTOM, HALF_WIDTH, BOLT_TOP)
    carved = outer.difference(seam).difference(chamber)
    stone = carved.difference(bolt)
    mono = carved.difference(bolt.buffer(MONO_GAP, join_style="mitre"))
    return _flip(stone), _flip(bolt), _flip(mono)
