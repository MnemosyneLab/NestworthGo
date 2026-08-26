import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

/** SampleForm keeps the shared React Hook Form + Zod example available for
 * component-level form tests. Feature forms use domain-specific schemas. */
const sampleSchema = z.object({
  name: z.string().trim().min(1, "sample.nameRequired"),
});

export type SampleFormValues = z.infer<typeof sampleSchema>;

export function SampleForm({ onSubmit }: { onSubmit: (values: SampleFormValues) => void }) {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<SampleFormValues>({
    resolver: zodResolver(sampleSchema),
    defaultValues: { name: "" },
  });

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-3" aria-label="Sample form">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="sample-name">Name</Label>
        <Input id="sample-name" {...register("name")} />
        {errors.name && (
          <p role="alert" className="text-xs text-destructive">
            {errors.name.message}
          </p>
        )}
      </div>
      <Button type="submit">Submit</Button>
    </form>
  );
}
