import { Box, HStack, Text, VStack } from "@chakra-ui/react";
import { useEffect, useRef } from "react";
import { useReducedMotion } from "~/hooks/useReducedMotion";
import { langyThinkingShimmerStyles } from "./langyShimmer";

/**
 * The model's live REASONING (thinking), shown while a turn streams and then
 * gone. It is deliberately EPHEMERAL: the store accumulates it from the
 * `reasoning` stream and clears it when the turn settles, so it is never part of
 * the durable answer and never reloads. This component only renders while a
 * reply is in flight.
 *
 * Quieter than the answer on purpose — reasoning is context, not the reply. It
 * reads as muted, smaller text under a shimmering "Thinking" header, in a capped
 * box that follows the live edge (older lines fade out the top) so a long think
 * never pushes the conversation around. Respects reduced motion.
 */
export function LangyReasoningStream({ reasoning }: { reasoning: string }) {
  const reduceMotion = useReducedMotion();
  const scrollRef = useRef<HTMLDivElement>(null);

  // Follow the live edge: keep the newest reasoning in view as it streams.
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [reasoning]);

  const shimmerCss = reduceMotion
    ? { ...langyThinkingShimmerStyles, animation: "none" }
    : langyThinkingShimmerStyles;

  return (
    <VStack align="stretch" gap={1.5} alignSelf="stretch" paddingLeft={0.5}>
      <HStack gap={2} align="center">
        <Box
          width="6px"
          height="6px"
          borderRadius="full"
          background="purple.solid"
          flexShrink={0}
        />
        <Text
          fontSize="13px"
          fontWeight="500"
          letterSpacing="-0.005em"
          css={shimmerCss}
          role="status"
          aria-live="off"
        >
          Thinking
        </Text>
      </HStack>
      <Box
        ref={scrollRef}
        maxHeight="8.5em"
        overflowY="auto"
        // Fade the top so lines scrolling out of view dissolve rather than clip.
        css={{
          maskImage:
            "linear-gradient(to bottom, transparent 0, black 1.5em, black 100%)",
          WebkitMaskImage:
            "linear-gradient(to bottom, transparent 0, black 1.5em, black 100%)",
          scrollbarWidth: "none",
          "&::-webkit-scrollbar": { display: "none" },
        }}
      >
        <Text
          textStyle="xs"
          color="fg.muted"
          whiteSpace="pre-wrap"
          lineHeight="1.55"
        >
          {reasoning}
        </Text>
      </Box>
    </VStack>
  );
}
