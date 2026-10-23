import { KIND_COLORS } from "@/lib/modeColors";
import type { Kind } from "@/lib/types";

/**
 * A side view of the vehicle, drawn after the real ones: Tallinn's low-floor
 * articulated trams (pantograph, three sections), its trolleybuses (two poles to
 * the wires), city buses (three doors), coach-like county buses, and Elron's
 * electric/diesel units (sloped cab). In the kind's colour; facing right.
 */
export function VehicleArt({ kind, className }: { kind: Kind; className?: string }) {
  const c = KIND_COLORS[kind];
  return (
    <svg viewBox="0 0 240 72" role="img" aria-hidden className={className}>
      {/* road / rail */}
      <line x1="4" y1="66" x2="236" y2="66" stroke="currentColor" strokeOpacity="0.25" strokeWidth="2" />
      {kind === "tram" && <Tram c={c} />}
      {kind === "trolley" && <Bus c={c} doors={3} poles />}
      {kind === "bus" && <Bus c={c} doors={3} />}
      {kind === "county" && <Coach c={c} />}
      {kind === "train" && <Train c={c} />}
    </svg>
  );
}

const GLASS = "#1e293b";
const WHEEL = "#111827";

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

function Windows({ from, to, y, h, gap = 3, n }: { from: number; to: number; y: number; h: number; gap?: number; n: number }) {
  const w = (to - from - gap * (n - 1)) / n;
  return (
    <>
      {Array.from({ length: n }, (_, i) => (
        <rect key={i} x={from + i * (w + gap)} y={y} width={w} height={h} rx="1.5" fill={GLASS} opacity="0.85" />
      ))}
    </>
  );
}

function Tram({ c }: { c: string }) {
  // Three low-floor sections with joints, pantograph on the middle one.
  return (
    <g>
      <path d="M118 22 l8 -9 h10 l-6 9" fill="none" stroke="#475569" strokeWidth="1.6" />
      <line x1="112" y1="13" x2="142" y2="13" stroke="#475569" strokeWidth="1.6" />
      {(
        [
          [10, 80],
          [84, 156],
          [160, 228],
        ] as const
      ).map(([a, b], i) => (
        <g key={a}>
          <path
            d={i === 2 ? `M${a} 22 h${b - a - 10} q10 0 10 12 v26 h-${b - a} z` : i === 0 ? `M${a + 10} 22 h${b - a - 10} v38 h-${b - a} v-26 q0 -12 10 -12 z` : `M${a} 22 h${b - a} v38 h-${b - a} z`}
            fill={c}
          />
          <rect x={a} y="50" width={b - a} height="4" fill="#ffffff" opacity="0.9" />
        </g>
      ))}
      <rect x="80" y="24" width="4" height="34" fill="#334155" />
      <rect x="156" y="24" width="4" height="34" fill="#334155" />
      <Windows from={16} to={74} y={27} h={15} n={3} />
      <Windows from={90} to={150} y={27} h={15} n={3} />
      <Windows from={166} to={214} y={27} h={15} n={2} />
      <path d="M216 27 h4 q6 0 6 8 v7 h-10 z" fill={GLASS} opacity="0.85" />
      <Wheels xs={[30, 62, 120, 194, 214]} y={62} r={4.5} />
    </g>
  );
}

function Bus({ c, doors, poles = false }: { c: string; doors: number; poles?: boolean }) {
  return (
    <g>
      {poles && (
        <>
          <line x1="70" y1="20" x2="200" y2="4" stroke="#475569" strokeWidth="1.8" />
          <line x1="76" y1="20" x2="206" y2="6" stroke="#475569" strokeWidth="1.8" />
          <rect x="62" y="17" width="22" height="4" rx="1" fill="#475569" />
        </>
      )}
      <rect x="150" y="16" width="30" height="5" rx="2" fill="#cbd5e1" />
      <path d="M14 22 h196 q14 0 16 12 l2 18 v8 h-214 z" fill={c} />
      <rect x="14" y="50" width="214" height="4" fill="#ffffff" opacity="0.9" />
      <Windows from={22} to={196} y={27} h={16} n={7} />
      <path d="M200 27 h12 q8 0 10 9 l1 6 h-23 z" fill={GLASS} opacity="0.85" />
      {Array.from({ length: doors }, (_, i) => {
        const x = [30, 110, 196][i] ?? 30;
        return <rect key={x} x={x} y="26" width="13" height="32" rx="1" fill="#e2e8f0" stroke={GLASS} strokeWidth="1" />;
      })}
      <Wheels xs={[44, 176]} />
    </g>
  );
}

function Coach({ c }: { c: string }) {
  // Higher floor, one door at the front, luggage bays below the windows.
  return (
    <g>
      <path d="M12 16 h196 q16 0 18 12 l2 24 v8 h-216 z" fill={c} />
      <rect x="12" y="42" width="216" height="3" fill="#ffffff" opacity="0.9" />
      <Windows from={20} to={190} y={21} h={17} gap={2} n={7} />
      <path d="M196 21 h14 q10 0 12 10 l1 7 h-27 z" fill={GLASS} opacity="0.85" />
      <rect x="198" y="40" width="13" height="18" rx="1" fill="#e2e8f0" stroke={GLASS} strokeWidth="1" />
      {[60, 100, 140].map((x) => (
        <rect key={x} x={x} y="48" width="30" height="9" rx="1" fill="none" stroke="#ffffff" strokeOpacity="0.6" strokeWidth="1" />
      ))}
      <Wheels xs={[40, 176]} />
    </g>
  );
}

function Train({ c }: { c: string }) {
  // Two cars of a low-floor unit with a sloped cab.
  return (
    <g>
      <path d="M150 18 l10 -8 h10 l-6 8" fill="none" stroke="#475569" strokeWidth="1.6" />
      <line x1="146" y1="10" x2="176" y2="10" stroke="#475569" strokeWidth="1.6" />
      <path d="M6 18 h104 v42 h-104 z" fill={c} />
      <path d="M114 18 h86 q22 0 30 24 l2 18 h-118 z" fill={c} />
      <rect x="110" y="20" width="4" height="38" fill="#334155" />
      <rect x="6" y="48" width="226" height="4" fill="#d1d5db" opacity="0.95" />
      <Windows from={12} to={104} y={24} h={15} n={4} />
      <Windows from={120} to={196} y={24} h={15} n={3} />
      <path d="M200 24 h4 q12 0 18 14 l1 3 h-23 z" fill={GLASS} opacity="0.85" />
      <rect x="54" y="24" width="12" height="32" rx="1" fill="#e2e8f0" stroke={GLASS} strokeWidth="1" />
      <rect x="150" y="24" width="12" height="32" rx="1" fill="#e2e8f0" stroke={GLASS} strokeWidth="1" />
      <Wheels xs={[20, 40, 104, 120, 196, 216]} y={63} r={4} />
    </g>
  );
}
