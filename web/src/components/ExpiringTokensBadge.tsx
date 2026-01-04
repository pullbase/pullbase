import { useState, useEffect } from 'react'
import { serversApi } from '../lib/api'
import { Badge } from './ui/badge'
import { AlertTriangle } from 'lucide-react'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from './ui/tooltip'

export default function ExpiringTokensBadge() {
  const [count, setCount] = useState<number>(0)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const fetchExpiringTokens = async () => {
      try {
        const response = await serversApi.getExpiringTokens(7)
        setCount(response.count)
      } catch (error) {
        console.error('Failed to fetch expiring tokens:', error)
      } finally {
        setIsLoading(false)
      }
    }

    fetchExpiringTokens()
    
    const interval = setInterval(fetchExpiringTokens, 60000)
    return () => clearInterval(interval)
  }, [])

  if (isLoading) return null
  if (count === 0) return null

  const handleClick = () => {
    console.log('Navigate to expiring tokens details')
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge 
            variant="destructive" 
            className="cursor-pointer gap-1.5 px-3 py-1 bg-amber-500 hover:bg-amber-600 border-amber-600 text-white animate-in fade-in zoom-in duration-300"
            onClick={handleClick}
          >
            <AlertTriangle className="w-3.5 h-3.5" />
            <span>{count} token{count !== 1 ? 's' : ''} expiring</span>
          </Badge>
        </TooltipTrigger>
        <TooltipContent>
          <p>{count} agent token{count !== 1 ? 's are' : ' is'} expiring within 7 days</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
