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
    <div className={`mb-3 flex flex-wrap items-center justify-between gap-2 ${className}`}>
      {/* 标题不许断行。窄栏里挤不下时，让右边那组按钮整体换到第二行，
          而不是把「设备列表」劈成两行；真放不下一整行才用省略号 */}
      <div className="min-w-0 truncate text-sm font-bold text-slate-800">{title}</div>
      {action ? <div className="ml-auto shrink-0">{action}</div> : null}
    </div>
  )
}
