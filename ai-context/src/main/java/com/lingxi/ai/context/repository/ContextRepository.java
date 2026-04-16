package com.lingxi.ai.context.repository;

import com.lingxi.ai.context.entity.Context;
import com.lingxi.ai.context.entity.ContextType;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.stereotype.Repository;

import java.util.List;

@Repository
public interface ContextRepository extends JpaRepository<Context, Long> {

    List<Context> findByTaskIdOrderByCreatedAtDesc(String taskId);

    List<Context> findByNodeIdAndContextTypeOrderByCreatedAtDesc(String nodeId, ContextType contextType);

    List<Context> findByTaskIdAndContextType(String taskId, ContextType contextType);
}
