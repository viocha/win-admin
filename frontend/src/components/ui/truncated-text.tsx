import { useLayoutEffect, useRef, useState, type ComponentProps } from "react"
import { cn } from "cn"

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"

type TruncatedTextProps = ComponentProps<"span"> & {
  value: string
  tooltipClassName?: string
}

function TruncatedText({
  value,
  className,
  tooltipClassName,
  ...props
}: TruncatedTextProps) {
  const textRef = useRef<HTMLSpanElement>(null)
  const [truncated, setTruncated] = useState(false)

  useLayoutEffect(() => {
    const element = textRef.current
    if (!element) return

    const update = () => {
      setTruncated(element.scrollWidth > element.clientWidth + 1)
    }

    update()
    const observer = new ResizeObserver(update)
    observer.observe(element)
    return () => observer.disconnect()
  }, [value])

  return (
    <span
      data-slot="truncated-text"
      className={cn("min-w-0 overflow-hidden whitespace-nowrap", className)}
      {...props}
    >
      <Tooltip disabled={!truncated}>
        <TooltipTrigger
          render={
            <span
              ref={textRef}
              className="inline-block max-w-full overflow-hidden text-ellipsis whitespace-nowrap align-bottom"
            >
              {value}
            </span>
          }
        />
        <TooltipContent className={cn("max-w-xl break-all whitespace-pre-wrap", tooltipClassName)}>
          {value}
        </TooltipContent>
      </Tooltip>
    </span>
  )
}

export { TruncatedText }
