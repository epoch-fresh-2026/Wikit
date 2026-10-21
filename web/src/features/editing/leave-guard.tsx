import { useCallback, useContext, type RefObject } from 'react'
import { UNSAFE_DataRouterContext, useBeforeUnload, useBlocker } from 'react-router'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from '@/components/ui/alert-dialog'

export function LeaveGuard({ dirty, saving }: { dirty: boolean; saving: RefObject<boolean> }) {
  useBeforeUnload(
    useCallback(
      (e) => {
        if (dirty && !saving.current) {
          e.preventDefault()
          e.returnValue = ''
        }
      },
      [dirty, saving],
    ),
  )
  const router = useContext(UNSAFE_DataRouterContext)
  return router ? <RouterGuard dirty={dirty} saving={saving} /> : null
}
function RouterGuard({ dirty, saving }: { dirty: boolean; saving: RefObject<boolean> }) {
  const blocker = useBlocker(useCallback(() => dirty && !saving.current, [dirty, saving]))
  return (
    <AlertDialog open={blocker.state === 'blocked'}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>放弃未提交的修改？</AlertDialogTitle>
          <AlertDialogDescription>离开后，本地修改将丢失。</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel onClick={() => blocker.state === 'blocked' && blocker.reset()}>
            继续编辑
          </AlertDialogCancel>
          <AlertDialogAction onClick={() => blocker.state === 'blocked' && blocker.proceed()}>
            放弃修改
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
