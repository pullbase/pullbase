import { Badge } from './ui/badge'
import { Check, X, HelpCircle, Loader2 } from 'lucide-react'
import { cn } from '../lib/utils'

export type ValidationStatus = 'valid' | 'invalid' | 'unknown' | 'pending'

interface ValidationStatusBadgeProps {
  status: ValidationStatus
  className?: string
}

export function ValidationStatusBadge({ status, className }: ValidationStatusBadgeProps) {
  const config = {
    valid: {
      icon: Check,
      text: 'Config Valid',
      className: 'bg-green-100 text-green-700 hover:bg-green-200 border-green-200 dark:bg-green-900/30 dark:text-green-400 dark:border-green-800',
      iconClass: 'text-green-600 dark:text-green-400'
    },
    invalid: {
      icon: X,
      text: 'Config Invalid',
      className: 'bg-red-100 text-red-700 hover:bg-red-200 border-red-200 dark:bg-red-900/30 dark:text-red-400 dark:border-red-800',
      iconClass: 'text-red-600 dark:text-red-400'
    },
    pending: {
      icon: Loader2,
      text: 'Validating',
      className: 'bg-blue-100 text-blue-700 hover:bg-blue-200 border-blue-200 dark:bg-blue-900/30 dark:text-blue-400 dark:border-blue-800',
      iconClass: 'text-blue-600 dark:text-blue-400 animate-spin'
    },
    unknown: {
      icon: HelpCircle,
      text: 'Validation Unknown',
      className: 'bg-gray-100 text-gray-700 hover:bg-gray-200 border-gray-200 dark:bg-gray-800 dark:text-gray-400 dark:border-gray-700',
      iconClass: 'text-gray-500 dark:text-gray-400'
    }
  }

  const { icon: Icon, text, className: badgeClass, iconClass } = config[status]

  return (
    <Badge variant="outline" className={cn(badgeClass, "gap-1.5", className)}>
      <Icon className={cn("h-3.5 w-3.5", iconClass)} />
      {text}
    </Badge>
  )
}
