interface SparklineProps {
  values: number[]
  stroke: string
  fill?: string
  className?: string
}

export function Sparkline({ values, stroke, fill, className = '' }: SparklineProps) {
  if (values.length === 0) return null
  const w = 100
  const h = 32
  const min = Math.min(...values)
  const max = Math.max(...values)
  const range = max - min || 1
  const stepX = w / (values.length - 1 || 1)
  const points = values.map((v, i) => {
    const x = i * stepX
    const y = h - ((v - min) / range) * h
    return [x, y] as const
  })
  const path = points.map(([x, y], i) => `${i === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`).join(' ')
  const area = `${path} L ${w} ${h} L 0 ${h} Z`
  return (
    <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" className={`h-8 w-full ${className}`}>
      {fill && <path d={area} fill={fill} opacity={0.35} />}
      <path d={path} stroke={stroke} strokeWidth={1.5} fill="none" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
