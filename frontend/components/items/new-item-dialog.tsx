"use client"

import * as React from "react"
import { useRouter } from "next/navigation"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useAuth } from "@/providers/auth-provider"
import { apiFetch, ApiError } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { TeamFieldSchema, WorkItem } from "@/types"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import { toast } from "sonner"
import { Loader2, Plus, Sparkles, AlertCircle } from "lucide-react"

interface NewItemDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultTeamId?: string
}

export function NewItemDialog({
  open,
  onOpenChange,
  defaultTeamId,
}: NewItemDialogProps) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { memberships, activeTeam } = useAuth()

  // Selected team (restricted to user's memberships)
  const initialTeam =
    defaultTeamId ||
    activeTeam?.team_id ||
    (memberships.length > 0 ? memberships[0].team_id : "")

  const [teamId, setTeamId] = React.useState<string>("")
  const effectiveTeamId = teamId || initialTeam

  const [title, setTitle] = React.useState("")
  const [description, setDescription] = React.useState("")
  const [priority, setPriority] = React.useState<number>(3)
  const [customFields, setCustomFields] = React.useState<Record<string, unknown>>({})
  const [errors, setErrors] = React.useState<Record<string, string>>({})

  // Idempotency key per intent
  const intentKeyRef = React.useRef<string>(crypto.randomUUID())

  // Reset intent key whenever core inputs change
  const handleInputChange = React.useCallback(() => {
    intentKeyRef.current = crypto.randomUUID()
    setErrors({})
  }, [])

  // Fetch field schemas for selected team
  const {
    data: schemas = [],
    isLoading: isLoadingSchemas,
  } = useQuery<TeamFieldSchema[]>({
    queryKey: queryKeys.teams.schemas(effectiveTeamId),
    queryFn: () => apiFetch<TeamFieldSchema[]>(`/teams/${effectiveTeamId}/field-schemas`),
    enabled: !!effectiveTeamId,
  })

  // Mutation to create work item
  const createMutation = useMutation({
    mutationFn: async () => {
      return await apiFetch<WorkItem>("/items", {
        method: "POST",
        idempotencyKey: intentKeyRef.current,
        body: JSON.stringify({
          team_id: effectiveTeamId,
          title: title.trim(),
          description: description.trim(),
          priority: Number(priority),
          custom_fields: customFields,
        }),
      })
    },
    onSuccess: (newItem) => {
      toast.success("Work item created successfully")
      queryClient.invalidateQueries({ queryKey: queryKeys.items.all })
      queryClient.invalidateQueries({ queryKey: queryKeys.views.counts() })
      queryClient.invalidateQueries({ queryKey: queryKeys.feed.all })

      // Reset form
      setTitle("")
      setDescription("")
      setPriority(3)
      setCustomFields({})
      setErrors({})
      intentKeyRef.current = crypto.randomUUID()

      onOpenChange(false)
      router.push(`/items/${newItem.id}`)
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        if (err.status === 422 || err.code === "validation_error") {
          const field = err.details?.field as string | undefined
          if (field) {
            setErrors((prev) => ({
              ...prev,
              [field]: err.message,
              ...(field.startsWith("custom_fields.")
                ? { [field.replace("custom_fields.", "")]: err.message }
                : {}),
            }))
          }
          toast.error(err.message || "Validation failed")
          return
        }
        if (err.status === 409 && err.code === "request_in_progress") {
          toast.error("Request already in progress. Please wait.")
          return
        }
        toast.error(err.message || "Failed to create work item")
      } else {
        toast.error(err instanceof Error ? err.message : "Network error occurred")
      }
    },
  })

  // Client validation mirroring server rules
  const validateForm = (): boolean => {
    const newErrors: Record<string, string> = {}

    if (!effectiveTeamId) {
      newErrors.team_id = "Please select a team"
    }
    if (!title.trim()) {
      newErrors.title = "Title is required"
    } else if (title.trim().length > 500) {
      newErrors.title = "Title must be at most 500 characters"
    }
    if (description.length > 30000) {
      newErrors.description = "Description must be at most 30000 characters"
    }
    if (![1, 2, 3, 4].includes(Number(priority))) {
      newErrors.priority = "Priority must be between 1 (P1) and 4 (P4)"
    }

    // Dynamic schema validation
    for (const schema of schemas) {
      const val = customFields[schema.field_key]
      if (schema.required) {
        if (val === undefined || val === null || val === "") {
          newErrors[schema.field_key] = `${schema.label} is required`
          continue
        }
      }
      if (val !== undefined && val !== null && val !== "") {
        const typeLower = schema.type.toLowerCase()
        if (typeLower === "number" || typeLower === "integer") {
          if (isNaN(Number(val))) {
            newErrors[schema.field_key] = `${schema.label} must be a number`
          }
        } else if (typeLower === "email") {
          if (typeof val !== "string" || !val.includes("@") || !val.includes(".")) {
            newErrors[schema.field_key] = `${schema.label} must be a valid email`
          }
        } else if (typeLower === "url") {
          try {
            new URL(String(val))
          } catch {
            newErrors[schema.field_key] = `${schema.label} must be a valid URL (http/https)`
          }
        }
      }
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!validateForm()) {
      toast.error("Please resolve validation errors before submitting")
      return
    }
    createMutation.mutate()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <div className="flex size-7 items-center justify-center rounded-md bg-primary/10 text-primary">
              <Sparkles className="size-4" />
            </div>
            <DialogTitle className="text-lg font-bold">Create Work Item</DialogTitle>
          </div>
          <DialogDescription className="text-xs text-muted-foreground">
            Initiate a new operational ticket, incident, or approval request.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4 py-2">
          {/* Team selector (User's memberships only) */}
          <div className="space-y-1.5">
            <Label className="text-xs font-semibold">
              Team <span className="text-destructive">*</span>
            </Label>
            {memberships.length === 0 ? (
              <p className="text-xs text-destructive">
                You do not belong to any teams. Contact an administrator.
              </p>
            ) : (
              <Select
                value={effectiveTeamId}
                onValueChange={(val) => {
                  setTeamId(val ?? "")
                  setCustomFields({})
                  handleInputChange()
                }}
              >
                <SelectTrigger className={errors.team_id ? "border-destructive" : ""}>
                  <SelectValue placeholder="Select a team" />
                </SelectTrigger>
                <SelectContent>
                  {memberships.map((m) => (
                    <SelectItem key={m.team_id} value={m.team_id}>
                      <span className="font-medium">{m.team_name}</span>{" "}
                      <span className="text-xs text-muted-foreground capitalize">
                        ({m.role})
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            {errors.team_id && (
              <p className="text-[11px] font-medium text-destructive flex items-center gap-1 mt-1">
                <AlertCircle className="size-3" />
                {errors.team_id}
              </p>
            )}
          </div>

          {/* Title */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label className="text-xs font-semibold">
                Title <span className="text-destructive">*</span>
              </Label>
              <span className="text-[10px] text-muted-foreground">
                {title.length}/500
              </span>
            </div>
            <Input
              placeholder="Brief summary of the issue or task..."
              value={title}
              maxLength={500}
              onChange={(e) => {
                setTitle(e.target.value)
                handleInputChange()
              }}
              className={errors.title ? "border-destructive" : ""}
            />
            {errors.title && (
              <p className="text-[11px] font-medium text-destructive flex items-center gap-1 mt-1">
                <AlertCircle className="size-3" />
                {errors.title}
              </p>
            )}
          </div>

          {/* Priority & SLA */}
          <div className="space-y-1.5">
            <Label className="text-xs font-semibold">
              Priority & Target SLA <span className="text-destructive">*</span>
            </Label>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {[
                { p: 1, label: "P1 Critical", sla: "1 hr SLA", color: "border-red-500/40 text-red-600 dark:text-red-400" },
                { p: 2, label: "P2 High", sla: "4 hrs SLA", color: "border-amber-500/40 text-amber-600 dark:text-amber-400" },
                { p: 3, label: "P3 Medium", sla: "24 hrs SLA", color: "border-blue-500/40 text-blue-600 dark:text-blue-400" },
                { p: 4, label: "P4 Low", sla: "72 hrs SLA", color: "border-slate-500/40 text-muted-foreground" },
              ].map(({ p, label, sla, color }) => {
                const isSelected = priority === p
                return (
                  <button
                    key={p}
                    type="button"
                    onClick={() => {
                      setPriority(p)
                      handleInputChange()
                    }}
                    className={`flex flex-col items-start p-2.5 rounded-md border text-left transition-all ${
                      isSelected
                        ? "border-primary bg-primary/10 ring-1 ring-primary"
                        : `hover:bg-muted/50 ${color}`
                    }`}
                  >
                    <span className="text-xs font-bold">{label}</span>
                    <span className="text-[10px] text-muted-foreground mt-0.5">{sla}</span>
                  </button>
                )
              })}
            </div>
            {errors.priority && (
              <p className="text-[11px] font-medium text-destructive flex items-center gap-1 mt-1">
                <AlertCircle className="size-3" />
                {errors.priority}
              </p>
            )}
          </div>

          {/* Description */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label className="text-xs font-semibold">Description</Label>
              <span className="text-[10px] text-muted-foreground">
                {description.length}/30000
              </span>
            </div>
            <Textarea
              placeholder="Provide background, steps to reproduce, or contextual logs..."
              rows={4}
              value={description}
              maxLength={30000}
              onChange={(e) => {
                setDescription(e.target.value)
                handleInputChange()
              }}
              className={errors.description ? "border-destructive" : ""}
            />
            {errors.description && (
              <p className="text-[11px] font-medium text-destructive flex items-center gap-1 mt-1">
                <AlertCircle className="size-3" />
                {errors.description}
              </p>
            )}
          </div>

          {/* Dynamic Custom Fields from GET /teams/:id/field-schemas */}
          {isLoadingSchemas ? (
            <div className="flex items-center gap-2 py-3 text-xs text-muted-foreground">
              <Loader2 className="size-3.5 animate-spin" />
              Loading team custom field schema...
            </div>
          ) : schemas.length > 0 ? (
            <div className="space-y-3 rounded-lg border bg-muted/20 p-3.5">
              <div className="flex items-center justify-between">
                <span className="text-xs font-bold uppercase tracking-wider text-muted-foreground">
                  Team Specific Fields
                </span>
                <Badge variant="outline" className="text-[10px]">
                  {schemas.length} {schemas.length === 1 ? "field" : "fields"}
                </Badge>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                {schemas.map((schema) => {
                  const val = customFields[schema.field_key]
                  const fieldErr = errors[schema.field_key]
                  const typeLower = schema.type.toLowerCase()

                  return (
                    <div key={schema.field_key} className="space-y-1.5">
                      <Label className="text-xs font-semibold">
                        {schema.label}
                        {schema.required && (
                          <span className="text-destructive ml-0.5">*</span>
                        )}
                      </Label>

                      {typeLower === "boolean" ? (
                        <div className="flex items-center gap-2 pt-1">
                          <input
                            type="checkbox"
                            id={`field-${schema.field_key}`}
                            checked={Boolean(val)}
                            onChange={(e) => {
                              setCustomFields((prev) => ({
                                ...prev,
                                [schema.field_key]: e.target.checked,
                              }))
                              handleInputChange()
                            }}
                            className="size-4 rounded border-gray-300 text-primary focus:ring-primary"
                          />
                          <label
                            htmlFor={`field-${schema.field_key}`}
                            className="text-xs text-muted-foreground cursor-pointer"
                          >
                            Enable / Yes
                          </label>
                        </div>
                      ) : typeLower === "select" && Array.isArray(schema.options) ? (
                        <Select
                          value={String(val ?? "")}
                          onValueChange={(opt) => {
                            setCustomFields((prev) => ({
                              ...prev,
                              [schema.field_key]: opt,
                            }))
                            handleInputChange()
                          }}
                        >
                          <SelectTrigger className={fieldErr ? "border-destructive" : ""}>
                            <SelectValue placeholder={`Select ${schema.label}`} />
                          </SelectTrigger>
                          <SelectContent>
                            {(schema.options as string[]).map((opt) => (
                              <SelectItem key={opt} value={opt}>
                                {opt}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        <Input
                          type={
                            typeLower === "number" || typeLower === "integer"
                              ? "number"
                              : typeLower === "date"
                              ? "date"
                              : typeLower === "email"
                              ? "email"
                              : typeLower === "url"
                              ? "url"
                              : "text"
                          }
                          placeholder={`Enter ${schema.label}...`}
                          value={val !== undefined && val !== null ? String(val) : ""}
                          onChange={(e) => {
                            const raw = e.target.value
                            const parsed =
                              typeLower === "number" || typeLower === "integer"
                                ? raw === "" ? null : Number(raw)
                                : raw
                            setCustomFields((prev) => ({
                              ...prev,
                              [schema.field_key]: parsed,
                            }))
                            handleInputChange()
                          }}
                          className={fieldErr ? "border-destructive" : ""}
                        />
                      )}

                      {fieldErr && (
                        <p className="text-[11px] font-medium text-destructive flex items-center gap-1 mt-0.5">
                          <AlertCircle className="size-3" />
                          {fieldErr}
                        </p>
                      )}
                    </div>
                  )
                })}
              </div>
            </div>
          ) : null}

          <DialogFooter className="gap-2 pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={createMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={createMutation.isPending || memberships.length === 0}
              className="gap-1.5"
            >
              {createMutation.isPending ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  Creating...
                </>
              ) : (
                <>
                  <Plus className="size-4" />
                  Create Work Item
                </>
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
