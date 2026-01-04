import { useRef, useEffect, useState } from 'react'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './ui/tooltip'
import { FormattedMessage } from './FormattedMessage'

interface SmartTooltipProps {
  content: string
  children: React.ReactNode
  className?: string
  side?: 'top' | 'right' | 'bottom' | 'left'
}

export function SmartTooltip({ content, children, className, side = 'top' }: SmartTooltipProps) {
  const textRef = useRef<HTMLDivElement>(null)
  const [isOverflowing, setIsOverflowing] = useState(true)

  useEffect(() => {
    const checkOverflow = () => {
      const element = textRef.current
      if (element) {
        const isTextOverflowing = element.scrollWidth > element.clientWidth || element.scrollHeight > element.clientHeight
        setIsOverflowing(isTextOverflowing)
      }
    }

    const timeoutId = setTimeout(checkOverflow, 100)
    
    window.addEventListener('resize', checkOverflow)
    
    return () => {
      clearTimeout(timeoutId)
      window.removeEventListener('resize', checkOverflow)
    }
  }, [content])

  // Always show tooltip for now to ensure it works, can be made smart later
  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          <div ref={textRef} className={`${className} ${isOverflowing ? 'cursor-help' : ''}`}>
            {children}
          </div>
        </TooltipTrigger>
        <TooltipContent
          side={side}
          className="max-w-lg break-words whitespace-normal p-4 bg-popover text-popover-foreground"
        >
          <FormattedMessage message={content} />
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
} 
