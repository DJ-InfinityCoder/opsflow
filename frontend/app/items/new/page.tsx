"use client"

import * as React from "react"
import { useRouter } from "next/navigation"
import { NewItemDialog } from "@/components/items/new-item-dialog"

export default function NewItemPage() {
  const router = useRouter()
  const [open, setOpen] = React.useState(true)

  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen)
    if (!isOpen) {
      router.push("/")
    }
  }

  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <NewItemDialog open={open} onOpenChange={handleOpenChange} />
    </div>
  )
}
