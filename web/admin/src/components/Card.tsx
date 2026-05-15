import type { HTMLAttributes, ReactNode } from 'react'

export function Card({
  className = '',
  children,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { children: ReactNode }) {
  return (
    <div
      {...rest}
      className={`rounded-2xl border border-slate-200/70 bg-white p-5 shadow-sm shadow-slate-200/60 ${className}`}
    >
      {children}
    </div>
  )
}

export function CardHeader({
  title,
  action,
  className = '',
}: {
  title: ReactNode
  action?: ReactNode
  className?: string
}) {
  return (
    <div className={`mb-3 flex items-center justify-between ${className}`}>
      <div className="text-sm font-bold text-slate-800">{title}</div>
      {action}
    </div>
  )
}
