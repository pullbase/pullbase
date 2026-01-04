import { Package, File, Settings, AlertCircle } from 'lucide-react'

interface FormattedMessageProps {
  message: string
}

export function FormattedMessage({ message }: FormattedMessageProps) {
  // If it's not a drift message, return as-is
  if (!message.includes('Configuration drift detected:')) {
    return <span>{message}</span>
  }

  // Parse the drift message
  const driftContent = message.replace('Configuration drift detected: ', '')
  const items = driftContent.split(', ')
  
  const parsedItems = items.map(item => {
    if (item.includes('Package') && item.includes('should be installed')) {
      const packageName = item.match(/Package (\w+)/)?.[1] || 'unknown'
      return {
        type: 'package',
        icon: Package,
        title: `Package: ${packageName}`,
        description: 'Should be installed but is absent'
      }
    } else if (item.includes('Package') && item.includes('should be removed')) {
      const packageName = item.match(/Package (\w+)/)?.[1] || 'unknown'
      return {
        type: 'package',
        icon: Package,
        title: `Package: ${packageName}`,
        description: 'Should be removed but is present'
      }
    } else if (item.includes('File')) {
      const filePath = item.match(/File '([^']+)'/)?.[1] || 'unknown'
      return {
        type: 'file',
        icon: File,
        title: `File: ${filePath}`,
        description: 'Content drift detected'
      }
    } else if (item.includes('Service')) {
      const serviceName = item.match(/Service (\w+)/)?.[1] || 'unknown'
      const desired = item.match(/desired running state '([^']+)'/)?.[1] || 'unknown'
      const actual = item.match(/actual state is (\w+)/)?.[1] || 'unknown'
      return {
        type: 'service',
        icon: Settings,
        title: `Service: ${serviceName}`,
        description: `Expected: ${desired}, Actual: ${actual}`
      }
    } else {
      return {
        type: 'other',
        icon: AlertCircle,
        title: 'Configuration Issue',
        description: item
      }
    }
  })

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 text-red-600 font-medium">
        <AlertCircle className="w-4 h-4" />
        <span>Configuration Drift Detected</span>
      </div>
      <div className="space-y-2">
        {parsedItems.map((item, index) => {
          const Icon = item.icon
          return (
            <div key={index} className="flex items-start gap-2 text-sm">
              <Icon className="w-4 h-4 mt-0.5 text-orange-500 flex-shrink-0" />
              <div>
                <div className="font-medium">{item.title}</div>
                <div className="text-sm text-popover-foreground">{item.description}</div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
} 