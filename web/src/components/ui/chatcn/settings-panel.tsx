import { Switch, Tabs } from "radix-ui"
import * as React from "react"
import { cn } from "@/lib/utils"

/* -------------------------------------------------------------------------------------------------
 * SettingsPanel
 *
 * Layout: a sections nav and one pane. At 42 rem and wider (a container query, so it follows the
 * space it is given, not the viewport) the nav is a column on the start side. Narrower, it becomes
 * a scrolling row above the pane. Panes swap instantly: people flip between sections often.
 * -----------------------------------------------------------------------------------------------*/

/** Below this width the nav sits above the pane and arrow keys move left and right. */
const STACK_BELOW = 672

interface SettingsPanelProps
  extends Omit<React.ComponentProps<typeof Tabs.Root>, "value" | "defaultValue" | "onValueChange"> {
  /** The open section. Controlled. */
  value?: string
  /** The section open at first. Uncontrolled. */
  defaultValue?: string
  onValueChange?: (value: string) => void
}

/**
 * The settings surface for an AI app: compose `SettingsNav` with `SettingsNavItem`s and one
 * `SettingsPane` per section. Give it a height (`className="h-[640px]"`) and each pane scrolls on
 * its own; without one it grows with its content.
 */
function SettingsPanel({
  value,
  defaultValue,
  onValueChange,
  className,
  children,
  ...props
}: SettingsPanelProps) {
  const ref = React.useRef<HTMLDivElement>(null)
  const [stacked, setStacked] = React.useState(false)

  // CSS decides the layout through a container query. This only keeps arrow keys in step with it.
  React.useLayoutEffect(() => {
    const el = ref.current
    if (!el || typeof ResizeObserver === "undefined") return
    const observer = new ResizeObserver(([entry]) => {
      if (entry) setStacked(entry.contentRect.width < STACK_BELOW)
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  return (
    <Tabs.Root
      activationMode="manual"
      ref={ref}
      value={value}
      defaultValue={defaultValue}
      onValueChange={(next) => onValueChange?.(String(next))}
      orientation={stacked ? "horizontal" : "vertical"}
      data-slot="settings-panel"
      className={cn("chatcn-text-14 text-chatcn-fg @container/settings flex min-h-0", className)}
      {...props}
    >
      <div className="flex min-h-0 w-full min-w-0 flex-col @2xl/settings:flex-row">{children}</div>
    </Tabs.Root>
  )
}

/* -------------------------------------------------------------------------------------------------
 * Nav
 * -----------------------------------------------------------------------------------------------*/

interface SettingsNavProps extends Omit<React.ComponentProps<"nav">, "title"> {
  /** Heading above the sections, usually "Settings". */
  title?: React.ReactNode
  /** Content pinned under the sections in the column layout: a plan summary or a sign out. */
  footer?: React.ReactNode
  /** Accessible name of the section list. Default "Settings sections". */
  label?: string
}

function SettingsNav({
  title,
  footer,
  label = "Settings sections",
  className,
  children,
  ...props
}: SettingsNavProps) {
  return (
    <nav
      aria-label={label}
      data-slot="settings-nav"
      className={cn(
        "border-chatcn-border flex shrink-0 flex-col border-b",
        "@2xl/settings:w-56 @2xl/settings:border-e @2xl/settings:border-b-0",
        className,
      )}
      {...props}
    >
      {title ? (
        <h2 className="chatcn-text-15 text-chatcn-fg px-4 pt-4 pb-1 font-medium tracking-[-0.005em] @2xl/settings:px-5 @2xl/settings:pt-5 @2xl/settings:pb-3">
          {title}
        </h2>
      ) : null}
      <Tabs.List
        data-slot="settings-nav-list"
        className={cn(
          // Row: scrolls sideways with the edge padding kept inside the scroller.
          "flex [scrollbar-width:none] gap-1 overflow-x-auto px-3 py-2 [&::-webkit-scrollbar]:hidden",
          "@2xl/settings:flex-col @2xl/settings:gap-px @2xl/settings:overflow-x-visible @2xl/settings:px-3 @2xl/settings:py-0",
        )}
      >
        {children}
      </Tabs.List>
      {footer ? (
        <div
          data-slot="settings-nav-footer"
          className="mt-auto hidden px-3 pt-3 pb-3 @2xl/settings:block"
        >
          {footer}
        </div>
      ) : null}
    </nav>
  )
}

interface SettingsNavItemProps extends Omit<React.ComponentProps<typeof Tabs.Trigger>, "value"> {
  value: string
  icon?: React.ReactNode
  /** A short fact after the label: a count, or "Off". Tabular. */
  meta?: React.ReactNode
}

function SettingsNavItem({
  value,
  icon,
  meta,
  className,
  children,
  ...props
}: SettingsNavItemProps) {
  return (
    <Tabs.Trigger
      value={value}
      data-slot="settings-nav-item"
      className={cn(
        "group/nav-item chatcn-focus rounded-chatcn-md flex h-8 shrink-0 items-center gap-2.5 px-2.5 text-start whitespace-nowrap select-none",
        "chatcn-text-13 text-chatcn-fg-muted @2xl/settings:chatcn-text-14 @2xl/settings:px-2",
        // Frequent and keyboard driven: color only, no movement, no transition on the active fill.
        "hover:bg-chatcn-fill-hover hover:text-chatcn-fg transition-colors duration-150",
        "data-[state=active]:bg-chatcn-fill data-[state=active]:text-chatcn-fg data-[state=active]:transition-none",
        "disabled:pointer-events-none disabled:opacity-50 pointer-coarse:h-11",
        className,
      )}
      {...props}
    >
      {icon ? (
        <span
          aria-hidden="true"
          className={cn(
            "text-chatcn-fg-subtle grid size-4 shrink-0 place-items-center [&_svg]:size-4",
            "group-hover/nav-item:text-chatcn-fg-muted group-data-[state=active]/nav-item:text-chatcn-fg",
          )}
        >
          {icon}
        </span>
      ) : null}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {meta !== undefined && meta !== null ? (
        <span className="chatcn-text-12 text-chatcn-fg-subtle hidden shrink-0 tabular-nums @2xl/settings:inline">
          {meta}
        </span>
      ) : null}
    </Tabs.Trigger>
  )
}

/** A hairline between groups of sections in the column layout. Hidden in the row layout. */
function SettingsNavSeparator({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      role="presentation"
      data-slot="settings-nav-separator"
      className={cn("bg-chatcn-border mx-2 my-2 hidden h-px @2xl/settings:block", className)}
      {...props}
    />
  )
}

/* -------------------------------------------------------------------------------------------------
 * Pane
 * -----------------------------------------------------------------------------------------------*/

interface SettingsPaneProps
  extends Omit<React.ComponentProps<typeof Tabs.Content>, "value" | "title"> {
  value: string
  title?: React.ReactNode
  description?: React.ReactNode
  /** Controls on the end side of the header, such as a Save button or a Learn more link. */
  actions?: React.ReactNode
  /** Classes for the inner column that holds the header and groups. */
  contentClassName?: string
}

function SettingsPane({
  value,
  title,
  description,
  actions,
  className,
  contentClassName,
  children,
  ...props
}: SettingsPaneProps) {
  return (
    <Tabs.Content
      value={value}
      data-slot="settings-pane"
      className={cn(
        "chatcn-scroll min-h-0 min-w-0 flex-1 overflow-y-auto outline-none",
        "focus-visible:outline-chatcn-accent focus-visible:outline-2 focus-visible:-outline-offset-2",
        className,
      )}
      {...props}
    >
      <div
        className={cn(
          "mx-auto flex w-full max-w-2xl flex-col gap-8 px-4 pt-5 pb-8 @2xl/settings:px-8 @2xl/settings:pt-7 @2xl/settings:pb-10",
          contentClassName,
        )}
      >
        {title || description || actions ? (
          <header className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              {title ? (
                <h2 className="chatcn-text-20 text-chatcn-fg font-medium tracking-[-0.01em] text-balance">
                  {title}
                </h2>
              ) : null}
              {description ? (
                <p className="chatcn-text-13 text-chatcn-fg-muted max-w-prose text-pretty">
                  {description}
                </p>
              ) : null}
            </div>
            {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
          </header>
        ) : null}
        {children}
      </div>
    </Tabs.Content>
  )
}

/* -------------------------------------------------------------------------------------------------
 * Group and row
 * -----------------------------------------------------------------------------------------------*/

interface SettingsGroupProps extends Omit<React.ComponentProps<"section">, "title"> {
  title?: React.ReactNode
  description?: React.ReactNode
  /** Something on the end side of the group heading, such as a Reset button. */
  action?: React.ReactNode
}

/**
 * A titled set of rows separated by hairlines. No card: the panel is already a surface, and
 * hairlines plus space carry the hierarchy.
 */
function SettingsGroup({
  title,
  description,
  action,
  className,
  children,
  ...props
}: SettingsGroupProps) {
  const id = React.useId()
  return (
    <section
      data-slot="settings-group"
      aria-labelledby={title ? id : undefined}
      className={cn("flex flex-col gap-2", className)}
      {...props}
    >
      {title || action ? (
        <div className="flex min-h-7 items-end justify-between gap-3">
          <div className="flex min-w-0 flex-col gap-0.5">
            {title ? (
              <h3 id={id} className="chatcn-text-13 text-chatcn-fg font-medium">
                {title}
              </h3>
            ) : null}
            {description ? (
              <p className="chatcn-text-12 text-chatcn-fg-subtle text-pretty">{description}</p>
            ) : null}
          </div>
          {action}
        </div>
      ) : null}
      <div
        data-slot="settings-group-rows"
        className="divide-chatcn-border border-chatcn-border flex flex-col divide-y border-t"
      >
        {children}
      </div>
    </section>
  )
}

interface SettingsRowContextValue {
  labelId: string
  descriptionId?: string
  disabled?: boolean
}

const SettingsRowContext = React.createContext<SettingsRowContextValue | null>(null)

/** Label and description ids of the nearest `SettingsRow`, so a control can name itself. */
function useSettingsRow() {
  return React.useContext(SettingsRowContext)
}

interface SettingsRowProps extends Omit<React.ComponentProps<"div">, "title"> {
  label: React.ReactNode
  description?: React.ReactNode
  /** Put the control under the text, full width: for text areas, lists and wide pickers. */
  stacked?: boolean
  disabled?: boolean
  /** A note under the row, such as "Managed by your admin". */
  note?: React.ReactNode
}

/**
 * One setting: label and description on the start side, the control on the end side. Wraps under
 * the text when space runs out. `SettingsSwitch` and `SettingsSegmented` inside a row name
 * themselves from its label and description.
 */
function SettingsRow({
  label,
  description,
  stacked = false,
  disabled = false,
  note,
  className,
  children,
  ...props
}: SettingsRowProps) {
  const labelId = React.useId()
  const descriptionId = React.useId()
  const value = React.useMemo(
    () => ({ labelId, descriptionId: description ? descriptionId : undefined, disabled }),
    [labelId, descriptionId, description, disabled],
  )
  return (
    <SettingsRowContext.Provider value={value}>
      <div
        data-slot="settings-row"
        data-disabled={disabled ? "" : undefined}
        className={cn(
          "flex min-h-14 flex-wrap items-center gap-x-6 gap-y-3 py-3.5",
          stacked && "flex-col items-stretch",
          className,
        )}
        {...props}
      >
        <div
          className={cn(
            "flex min-w-[min(100%,16rem)] flex-1 flex-col gap-0.5",
            disabled && "opacity-50",
          )}
        >
          <span id={labelId} className="chatcn-text-14 text-chatcn-fg font-medium">
            {label}
          </span>
          {description ? (
            <span id={descriptionId} className="chatcn-text-13 text-chatcn-fg-muted text-pretty">
              {description}
            </span>
          ) : null}
          {note ? <span className="chatcn-text-12 text-chatcn-fg-subtle pt-1">{note}</span> : null}
        </div>
        {children !== undefined && children !== null ? (
          <div
            data-slot="settings-row-control"
            className={cn("flex shrink-0 items-center gap-2", stacked && "w-full")}
          >
            {children}
          </div>
        ) : null}
      </div>
    </SettingsRowContext.Provider>
  )
}

/* -------------------------------------------------------------------------------------------------
 * Switch
 * -----------------------------------------------------------------------------------------------*/

type SettingsSwitchProps = React.ComponentProps<typeof Switch.Root>

/**
 * An on/off control. The thumb slides 150 ms and the track fills with ink, so the state never
 * depends on color alone. Inside a `SettingsRow` it takes the row's label and description.
 */
function SettingsSwitch({ className, disabled, ...props }: SettingsSwitchProps) {
  const row = useSettingsRow()
  return (
    <Switch.Root
      aria-labelledby={props["aria-label"] ? undefined : row?.labelId}
      aria-describedby={row?.descriptionId}
      disabled={disabled ?? row?.disabled}
      data-slot="settings-switch"
      className={cn(
        "chatcn-focus relative inline-flex h-[1.125rem] w-8 shrink-0 cursor-pointer items-center rounded-full p-0.5",
        "bg-chatcn-fill-active data-[state=checked]:bg-chatcn-accent",
        "ease-chatcn-standard transition-[background-color] duration-150",
        "data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50",
        // 44 px of hit area on touch without changing the drawing.
        "pointer-coarse:after:absolute pointer-coarse:after:-inset-3 pointer-coarse:after:content-['']",
        "forced-colors:outline-1 forced-colors:outline-solid",
        className,
      )}
      {...props}
    >
      <Switch.Thumb
        className={cn(
          "bg-chatcn-card shadow-chatcn-xs block size-3.5 rounded-full",
          "ease-chatcn-out transition-[translate,background-color] duration-150 motion-reduce:transition-none",
          "data-[state=checked]:bg-chatcn-accent-fg data-[state=checked]:translate-x-3.5 rtl:data-[state=checked]:-translate-x-3.5",
        )}
      />
    </Switch.Root>
  )
}

/* -------------------------------------------------------------------------------------------------
 * Segmented
 * -----------------------------------------------------------------------------------------------*/

interface SettingsSegmentedOption {
  value: string
  label: React.ReactNode
  icon?: React.ReactNode
  /** Show only the icon; `label` becomes the accessible name. */
  iconOnly?: boolean
  disabled?: boolean
}

interface SettingsSegmentedProps
  extends Omit<React.ComponentProps<"div">, "onChange" | "defaultValue"> {
  options: SettingsSegmentedOption[]
  value?: string
  defaultValue?: string
  onValueChange?: (value: string) => void
  disabled?: boolean
  size?: "xs" | "sm"
}

/**
 * Two to four exclusive choices in one track. Native radio inputs underneath: Tab enters the
 * group, arrow keys move and select, and screen readers hear a radio group. The selection moves
 * instantly; it is changed often and from the keyboard.
 */
function SettingsSegmented({
  options,
  value: valueProp,
  defaultValue,
  onValueChange,
  disabled: disabledProp,
  size = "sm",
  className,
  ...props
}: SettingsSegmentedProps) {
  const row = useSettingsRow()
  const disabled = disabledProp ?? row?.disabled
  const [valueState, setValueState] = React.useState(defaultValue ?? options[0]?.value ?? "")
  const value = valueProp ?? valueState
  const name = React.useId()
  return (
    <div
      role="radiogroup"
      aria-labelledby={props["aria-label"] ? undefined : row?.labelId}
      aria-describedby={row?.descriptionId}
      aria-disabled={disabled || undefined}
      data-slot="settings-segmented"
      data-size={size}
      className={cn(
        "bg-chatcn-fill inline-flex shrink-0 items-center gap-0.5 p-0.5",
        size === "xs" ? "rounded-chatcn-sm" : "rounded-chatcn-md",
        disabled && "opacity-50",
        className,
      )}
      {...props}
    >
      {options.map((option) => {
        const checked = option.value === value
        return (
          <label
            key={option.value}
            data-checked={checked ? "" : undefined}
            className={cn(
              "relative inline-flex cursor-pointer items-center justify-center gap-1.5 font-medium whitespace-nowrap select-none",
              size === "xs"
                ? "chatcn-text-12 rounded-chatcn-xs h-6 px-2 [&_svg]:size-3.5"
                : "chatcn-text-13 rounded-chatcn-sm h-7 px-2.5 [&_svg]:size-4",
              option.iconOnly && (size === "xs" ? "w-6 px-0" : "w-8 px-0"),
              "text-chatcn-fg-muted hover:text-chatcn-fg transition-colors duration-150",
              // Fixed: the label sets data-checked, not data-state (upstream chatcn bug).
              "data-checked:bg-chatcn-card data-checked:text-chatcn-fg data-checked:shadow-chatcn-xs data-checked:transition-none",
              "has-[:focus-visible]:outline-chatcn-accent has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-1",
              "has-[:disabled]:pointer-events-none has-[:disabled]:opacity-50",
              "pointer-coarse:min-h-11",
              "forced-colors:data-checked:outline-1 forced-colors:data-checked:outline-solid",
            )}
          >
            <input
              type="radio"
              name={name}
              value={option.value}
              checked={checked}
              disabled={disabled || option.disabled}
              aria-label={
                option.iconOnly && typeof option.label === "string" ? option.label : undefined
              }
              onChange={() => {
                if (valueProp === undefined) setValueState(option.value)
                onValueChange?.(option.value)
              }}
              className="sr-only"
            />
            {option.icon ? (
              <span aria-hidden="true" className="grid place-items-center">
                {option.icon}
              </span>
            ) : null}
            {option.iconOnly ? null : <span>{option.label}</span>}
          </label>
        )
      })}
    </div>
  )
}

export type {
  SettingsGroupProps,
  SettingsNavItemProps,
  SettingsNavProps,
  SettingsPanelProps,
  SettingsPaneProps,
  SettingsRowProps,
  SettingsSegmentedOption,
  SettingsSegmentedProps,
  SettingsSwitchProps,
}
export {
  SettingsGroup,
  SettingsNav,
  SettingsNavItem,
  SettingsNavSeparator,
  SettingsPane,
  SettingsPanel,
  SettingsRow,
  SettingsSegmented,
  SettingsSwitch,
  useSettingsRow,
}
