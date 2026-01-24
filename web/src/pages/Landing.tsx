import Features from "@/components/Features";
import Hero from "@/components/Hero";
import Install from "@/components/Install";

export default function Landing() {
  return (
    <div class="flex flex-col">
      <Hero />
      <Features />
      <Install />
    </div>
  );
}
