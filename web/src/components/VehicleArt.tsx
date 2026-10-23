import { LIVERY_COLORS, type Livery, type LiveryColors } from "@/lib/livery";

/**
 * A side view of the vehicle in its operator's current livery (see lib/livery):
 * TLT's Pesa/CAF trams, Solaris buses and Škoda trolleybuses (white, black window
 * band, slanted skirt blocks and a dashed roof line in the mode's colour), GoBus's
 * red Tartu Scanias with the white skyline, Elron's orange-nosed units and the
 * white county coaches. Facing right.
 */
export function VehicleArt({ livery, className }: { livery: Livery; className?: string }) {
  const c = LIVERY_COLORS[livery];
  return (
    <svg viewBox="0 0 240 72" role="img" aria-hidden className={className}>
      {/* road / rail */}
      <line x1="4" y1="66" x2="236" y2="66" stroke="currentColor" strokeOpacity="0.25" strokeWidth="2" />
      {livery === "tlt-tram" && <Tram c={c} />}
      {livery === "tlt-trolley" && <CityBus c={c} poles />}
      {livery === "tlt-bus" && <CityBus c={c} gas />}
      {livery === "tartu" && <TartuBus c={c} />}
      {livery === "county" && <Coach c={c} />}
      {livery === "elron" && <Train c={c} />}
    </svg>
  );
}

const OUTLINE = "#94a3b8";
const WHEEL = "#111827";
const POLE = "#475569";

function Wheels({ xs, y = 62, r = 5.5 }: { xs: number[]; y?: number; r?: number }) {
  return (
    <>
      {xs.map((x) => (
        <g key={x}>
          <circle cx={x} cy={y} r={r} fill={WHEEL} />
          <circle cx={x} cy={y} r={r * 0.4} fill="#9ca3af" />
        </g>
      ))}
    </>
  );
}

/** TLT's slanted skirt blocks between x0 and x1, leaning forward. */
function Blocks({ x0, x1, y, h, w, gap, fill }: { x0: number; x1: number; y: number; h: number; w: number; gap: number; fill: string }) {
  const n = Math.floor((x1 - x0 + gap) / (w + gap));
  return (
    <>
      {Array.from({ length: n }, (_, i) => {
        const x = x0 + i * (w + gap);
        return <path key={i} d={`M${x + 4} ${y} h${w} l-4 ${h} h-${w} z`} fill={fill} />;
      })}
    </>
  );
}

/** TLT's dashed line under the roof. */
function RoofDashes({ x0, x1, y, fill }: { x0: number; x1: number; y: number; fill: string }) {
  const n = Math.floor((x1 - x0) / 7);
  return (
    <>
      {Array.from({ length: n }, (_, i) => (
        <rect key={i} x={x0 + i * 7} y={y} width="4" height="2" fill={fill} />
      ))}
    </>
  );
}

/** Door panels: glass down to the floor, framed by the body colour. */
function Doors({ xs, y, h, w = 13, c }: { xs: number[]; y: number; h: number; w?: number; c: LiveryColors }) {
  return (
    <>
      {xs.map((x) => (
        <g key={x}>
          <rect x={x} y={y} width={w} height={h} rx="1" fill={c.band} stroke={c.body} strokeWidth="1.4" />
          <line x1={x + w / 2} y1={y + 2} x2={x + w / 2} y2={y + h - 2} stroke={c.body} strokeWidth="0.7" opacity="0.5" />
        </g>
      ))}
    </>
  );
}

function Tram({ c }: { c: LiveryColors }) {
  // Low-floor articulated tram: three sections, pantograph in the middle, red nose.
  const sections = [
    [10, 80],
    [84, 156],
    [160, 228],
  ] as const;
  return (
    <g>
      <path d="M118 21 l8 -9 h10 l-6 9" fill="none" stroke={POLE} strokeWidth="1.6" />
      <line x1="112" y1="12" x2="142" y2="12" stroke={POLE} strokeWidth="1.6" />
      {sections.map(([a, b], i) => (
        <path
          key={a}
          d={
            i === 2
              ? `M${a} 21 h${b - a - 12} q12 0 12 14 v25 h-${b - a} z`
              : i === 0
                ? `M${a + 8} 21 h${b - a - 8} v39 h-${b - a} v-29 q0 -10 8 -10 z`
                : `M${a} 21 h${b - a} v39 h-${b - a} z`
          }
          fill={c.body}
          stroke={OUTLINE}
          strokeWidth="0.8"
        />
      ))}
      <rect x="80" y="23" width="4" height="35" fill="#334155" />
      <rect x="156" y="23" width="4" height="35" fill="#334155" />
      {/* black window band, the cab glass running round the front */}
      <rect x="12" y="27" width="68" height="17" fill={c.band} />
      <rect x="84" y="27" width="72" height="17" fill={c.band} />
      <path d="M160 27 h56 q9 0 11 10 v7 h-67 z" fill={c.band} />
      <Doors xs={[30, 112, 182]} y={27} h={31} w={12} c={c} />
      <RoofDashes x0={14} x1={80} y={23} fill={c.accent} />
      <RoofDashes x0={86} x1={156} y={23} fill={c.accent} />
      <RoofDashes x0={162} x1={214} y={23} fill={c.accent} />
      <Blocks x0={12} x1={78} y={50} h={8} w={11} gap={10} fill={c.accent} />
      <Blocks x0={86} x1={154} y={50} h={8} w={11} gap={10} fill={c.accent} />
      <Blocks x0={162} x1={210} y={50} h={8} w={11} gap={10} fill={c.accent} />
      <path d="M214 48 h14 v12 h-18 z" fill={c.accent} />
      <Wheels xs={[30, 62, 120, 194, 214]} y={62} r={4} />
    </g>
  );
}

function CityBus({ c, poles = false, gas = false }: { c: LiveryColors; poles?: boolean; gas?: boolean }) {
  // Low-floor city bus with three doors; gas tanks on the roof (CNG Solaris) or
  // trolley poles to the wires (Škoda), whose front mask is dark.
  return (
    <g>
      {poles && (
        <>
          <line x1="70" y1="19" x2="200" y2="3" stroke={POLE} strokeWidth="1.8" />
          <line x1="76" y1="19" x2="206" y2="5" stroke={POLE} strokeWidth="1.8" />
          <rect x="62" y="16" width="22" height="4" rx="1" fill={POLE} />
        </>
      )}
      {gas && <path d="M120 21 v-4 q0 -3 4 -3 h76 q6 0 8 7 z" fill={c.body} stroke={OUTLINE} strokeWidth="0.8" />}
      <path d="M14 21 h196 q14 0 16 12 l2 19 v8 h-214 z" fill={c.body} stroke={OUTLINE} strokeWidth="0.8" />
      <rect x="18" y="26" width="184" height="18" fill={c.band} />
      <path d="M200 26 h12 q9 0 11 10 l1 8 h-24 z" fill={c.band} />
      {poles && <path d="M212 44 h12 l3 8 v8 h-15 z" fill={c.band} />}
      <RoofDashes x0={20} x1={206} y={22} fill={c.accent} />
      <Blocks x0={46} x1={104} y={47} h={12} w={9} gap={9} fill={c.accent} />
      <Blocks x0={124} x1={186} y={47} h={12} w={9} gap={9} fill={c.accent} />
      <path d="M14 46 h22 l-5 14 h-17 z" fill={c.accent} />
      <Doors xs={[34, 108, 196]} y={26} h={32} c={c} />
      <Wheels xs={[54, 176]} />
    </g>
  );
}

function TartuBus({ c }: { c: LiveryColors }) {
  // GoBus Scania in Tartu: red below the windows with the city's skyline in white.
  return (
    <g>
      <path d="M120 21 v-4 q0 -3 4 -3 h76 q6 0 8 7 z" fill={c.body} stroke={OUTLINE} strokeWidth="0.8" />
      <path d="M14 21 h196 q14 0 16 12 l2 19 v8 h-214 z" fill={c.body} stroke={OUTLINE} strokeWidth="0.8" />
      <path d="M14 43 h214 v9 l0 8 h-214 z" fill={c.accent} />
      <rect x="18" y="25" width="184" height="18" fill={c.band} />
      <path d="M200 25 h12 q9 0 11 10 l1 8 h-24 z" fill={c.band} />
      {/* skyline: town hall, church spire, the arch bridge, houses */}
      <path
        d="M50 58 v-8 h4 v-4 l2 -2 l2 2 v4 h4 v8 M66 58 v-6 h6 v6 M120 58 v-6 h3 v-6 l2 -5 l2 5 v6 h3 v6 M134 58 v-4 h8 v4 M146 58 q10 -10 20 0 M150 58 v-5 M162 58 v-5 M172 58 v-7 l4 -3 l4 3 v7"
        fill="none"
        stroke={c.body}
        strokeWidth="1"
      />
      <Doors xs={[34, 108, 196]} y={25} h={33} c={c} />
      <Wheels xs={[54, 176]} />
    </g>
  );
}

function Coach({ c }: { c: LiveryColors }) {
  // Higher-floor county coach, one door at the front, luggage bays below the windows.
  return (
    <g>
      <path d="M12 16 h196 q16 0 18 12 l2 24 v8 h-216 z" fill={c.body} stroke={OUTLINE} strokeWidth="0.8" />
      <rect x="18" y="21" width="174" height="17" rx="1.5" fill={c.band} />
      <path d="M196 21 h14 q10 0 12 10 l1 7 h-27 z" fill={c.band} />
      <rect x="12" y="42" width="216" height="2.5" fill={c.accent} />
      <rect x="198" y="40" width="13" height="18" rx="1" fill={c.band} />
      {[60, 100, 140].map((x) => (
        <rect key={x} x={x} y="48" width="30" height="9" rx="1" fill="none" stroke={OUTLINE} strokeWidth="1" />
      ))}
      <Wheels xs={[40, 176]} />
    </g>
  );
}

function Train({ c }: { c: LiveryColors }) {
  // Two cars of Elron's unit: orange nose and skirt, white sides with black
  // swooshes and the cyan line sweeping up to the cab.
  return (
    <g>
      <path d="M150 17 l10 -8 h10 l-6 8" fill="none" stroke={POLE} strokeWidth="1.6" />
      <line x1="146" y1="9" x2="176" y2="9" stroke={POLE} strokeWidth="1.6" />
      <path d="M6 17 h104 v43 h-104 z" fill={c.body} stroke={OUTLINE} strokeWidth="0.8" />
      <path d="M114 17 h86 q22 0 30 24 l2 19 h-118 z" fill={c.body} stroke={OUTLINE} strokeWidth="0.8" />
      <rect x="110" y="19" width="4" height="39" fill="#334155" />
      <rect x="8" y="24" width="100" height="15" fill={c.band} />
      <rect x="116" y="24" width="80" height="15" fill={c.band} />
      {/* orange cab and skirt */}
      <path d="M196 17 h4 q22 0 30 24 l2 19 h-44 l6 -19 z" fill={c.accent} />
      <path d="M200 24 h4 q12 0 18 14 l1 3 h-23 z" fill={c.band} />
      <rect x="6" y="54" width="104" height="6" fill={c.accent} />
      <rect x="114" y="54" width="74" height="6" fill={c.accent} />
      {/* swooshes and the cyan line */}
      <path d="M150 52 L194 40 L194 44 L156 52 z M166 52 L194 46 L194 49 L172 52 z" fill={c.band} />
      <path d="M114 50 h44 L190 42 v2 L160 52 h-46 z" fill={c.line} />
      <path d="M6 50 h104 v2 h-104 z" fill={c.line} />
      <Doors xs={[54, 140]} y={24} h={30} w={12} c={c} />
      <Wheels xs={[20, 40, 104, 120, 196, 216]} y={63} r={4} />
    </g>
  );
}
